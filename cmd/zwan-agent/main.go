// Command zwan-agent is the Windows client that joins one or more networks.
//
// Modes:
//
//	(default)   M1a: control-plane join only (register + peer list).
//	--tun       M1b-1: also create the adapter and assign the overlay IP.
//	--up        M1b-2: also start a wireguard-go tunnel to peers (direct path).
//
// --tun and --up require Administrator.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/l4"
	"github.com/Zeliper/zwan/client/resolver"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/client/wgbind"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

func main() {
	server := flag.String("server", "https://127.0.0.1:8787", "control server URL (may carry the key pin as \"#sha256:...\")")
	pin := flag.String("pin", "", "control server key fingerprint; required when the server has no CA-issued certificate")
	token := flag.String("token", "", "join token")
	device := flag.String("device", "", "stable device UUID")
	name := flag.String("name", "", "hostname label (defaults to OS hostname)")
	useTun := flag.Bool("tun", false, "create the adapter and assign the overlay IP (Administrator)")
	useUp := flag.Bool("up", false, "create the adapter and start the WireGuard tunnel to peers (Administrator)")
	useRelay := flag.Bool("relay", false, "route the tunnel through the server relay instead of direct endpoints")
	adapter := flag.String("adapter", "", "adapter name (default \"<Product>-<network>\")")
	wgPort := flag.Int("wg-port", 51820, "local WireGuard UDP listen port (direct mode)")
	endpoint := flag.String("endpoint", "", "endpoint host:port peers use to reach us (default 127.0.0.1:<wg-port>)")
	dnsAddr := flag.String("dns-addr", "127.0.0.1:53", "local DNS resolver listen address (with --up; empty to disable)")
	publishName := flag.String("publish-name", "", "publish a service of this name hosted on this node")
	publishPort := flag.Int("publish-port", 0, "published service port (over the overlay)")
	publishProto := flag.String("publish-proto", "tcp", "published service protocol (tcp/udp)")
	publishBackend := flag.Int("publish-backend-port", 0, "localhost backend port; enables the L4 proxy (0 = service binds the overlay itself)")
	flag.Parse()

	log.Printf("%s (%s) %s", shared.ProductName, shared.ComponentAgent, shared.Version)
	if *token == "" || *device == "" {
		log.Fatal("--token and --device are required")
	}
	hostname := *name
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	ep := *endpoint
	if ep == "" {
		ep = fmt.Sprintf("127.0.0.1:%d", *wgPort)
	}

	cl, err := join.NewClient(*server, *pin)
	if err != nil {
		log.Fatalf("control server: %v", err)
	}
	log.Printf("control server: %s (%s)", cl.BaseURL(), trustLabel(cl))
	res, err := cl.Join(*token, *device, hostname, ep)
	if err != nil {
		log.Fatalf("join failed: %v", err)
	}
	log.Printf("joined: network=%s suffix=%s assigned_ip=%s",
		res.Register.NetworkID, res.Register.DNSSuffix, res.Register.AssignedIP)
	log.Printf("node public key: %s", res.PublicKey)
	printPeers(res.Peers)

	if *publishName != "" {
		if *publishPort == 0 {
			log.Fatal("--publish-port is required with --publish-name")
		}
		svc := proto.Service{Name: *publishName, Proto: *publishProto, Port: *publishPort, BackendPort: *publishBackend, NodeIP: res.Register.AssignedIP}
		if err := cl.PublishService(svc); err != nil {
			log.Fatalf("publish service: %v", err)
		}
		log.Printf("published service %s.%s -> %s:%d/%s (backend 127.0.0.1:%d)",
			svc.Name, res.Register.DNSSuffix, svc.NodeIP, svc.Port, svc.Proto, svc.BackendPort)
	}

	adapterName := *adapter
	if adapterName == "" {
		adapterName = fmt.Sprintf("%s-%s", shared.ProductName, res.Register.NetworkID)
	}

	switch {
	case *useUp:
		runTunnel(cl, res, adapterName, *wgPort, *useRelay, *dnsAddr)
	case *useTun:
		bringUpAdapter(res, adapterName)
	default:
		// control-plane only; nothing more to do.
	}
}

func printPeers(peers []proto.Peer) {
	log.Printf("peers (%d):", len(peers))
	for _, p := range peers {
		log.Printf("  %-15s %-12s ep=%-21s %s", p.AssignedIP, p.Hostname, p.Endpoint, p.PublicKey)
	}
}

// bringUpAdapter (M1b-1): create the adapter and assign the node IP only.
func bringUpAdapter(res *join.Result, adapterName string) {
	log.Printf("creating adapter %q ...", adapterName)
	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		log.Fatalf("adapter: %v (Administrator? wintun.dll present?)", err)
	}
	defer closeAdapter(ad, adapterName)

	if err := ad.SetNodeIP(res.Register.AssignedIP); err != nil {
		log.Fatalf("assign overlay IP: %v", err)
	}
	log.Printf("adapter %q up with %s. Ctrl+C to remove.", adapterName, res.Register.AssignedIP)
	waitForInterrupt()
}

// runTunnel (M1b-2/M1b-3): adapter + wireguard-go device + periodic peer refresh.
// With useRelay, the tunnel is carried through the server relay (M1b-3); otherwise
// it uses direct peer endpoints (M1b-2).
func runTunnel(cl *join.Client, res *join.Result, adapterName string, wgPort int, useRelay bool, dnsAddr string) {
	log.Printf("creating adapter %q ...", adapterName)
	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		log.Fatalf("adapter: %v (Administrator? wintun.dll present?)", err)
	}
	defer closeAdapter(ad, adapterName)

	if err := ad.SetNodeIP(res.Register.AssignedIP); err != nil {
		log.Fatalf("assign overlay IP: %v", err)
	}

	var dev *wg.Device
	if useRelay {
		if res.Register.RelayAddr == "" {
			log.Fatal("--relay set but the server did not advertise a relay address")
		}
		selfIP, err := netip.ParseAddr(res.Register.AssignedIP)
		if err != nil {
			log.Fatalf("parse assigned IP: %v", err)
		}
		bind, err := wgbind.NewRelay(res.Register.RelayAddr, selfIP)
		if err != nil {
			log.Fatalf("relay bind: %v", err)
		}
		dev, err = wg.Up(ad, res.Private, wgPort, bind)
		if err != nil {
			log.Fatalf("wireguard: %v", err)
		}
		log.Printf("tunnel up on %q (%s) via relay %s. Ctrl+C to stop.", adapterName, res.Register.AssignedIP, res.Register.RelayAddr)
	} else {
		dev, err = wg.Up(ad, res.Private, wgPort, wgbind.New())
		if err != nil {
			log.Fatalf("wireguard: %v", err)
		}
		log.Printf("tunnel up on %q (%s), wg-port %d, direct. Ctrl+C to stop.", adapterName, res.Register.AssignedIP, wgPort)
	}
	defer dev.Close()

	var rslv *resolver.Resolver
	if dnsAddr != "" {
		rslv = resolver.New(res.Register.DNSSuffix)
		bound, err := rslv.Listen(dnsAddr)
		if err != nil {
			log.Printf("dns resolver: %v (continuing without local DNS)", err)
			rslv = nil
		} else {
			go func() { _ = rslv.Serve() }()
			defer rslv.Shutdown()
			log.Printf("dns resolver on %s for *.%s", bound, res.Register.DNSSuffix)
		}
	}

	stop := make(chan struct{})
	go refreshPeers(cl, dev, ad, rslv, res.Register.DNSSuffix, res.Register.AssignedIP, useRelay, stop)
	waitForInterrupt()
	close(stop)
}

// refreshPeers polls the control server and reprograms the tunnel + routes as
// peers appear or change.
//
// The WireGuard peer set is only re-applied when it actually changes: re-sending
// replace_peers on every tick would reset in-flight handshakes (which take a few
// seconds) and they would never complete.
func refreshPeers(cl *join.Client, dev *wg.Device, ad *tun.Adapter, rslv *resolver.Resolver, suffix, selfIP string, useRelay bool, stop <-chan struct{}) {
	routed := map[string]bool{}
	hostProxies := map[string]*l4.TCPProxy{}
	var lastSig string
	var curPeers []wg.Peer
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	apply := func() {
		peers, err := cl.Peers()
		if err != nil {
			log.Printf("peer refresh: %v", err)
			return
		}
		var wgPeers []wg.Peer
		for _, p := range peers {
			if p.AssignedIP == selfIP || p.PublicKey == "" {
				continue // skip self / incomplete
			}
			hexKey, err := keys.PublicHexFromBase64(p.PublicKey)
			if err != nil {
				log.Printf("peer %s: bad public key: %v", p.AssignedIP, err)
				continue
			}
			// In relay mode the WireGuard endpoint is the peer's overlay IP
			// (the relay routes by it); in direct mode it is the peer's UDP endpoint.
			endpoint := p.Endpoint
			if useRelay {
				endpoint = p.AssignedIP
			}
			wgPeers = append(wgPeers, wg.Peer{
				PublicKeyHex: hexKey,
				Endpoint:     endpoint,
				AllowedIP:    p.AssignedIP + "/32",
			})
			if !routed[p.AssignedIP] {
				if err := ad.AddPeerRoute(p.AssignedIP); err != nil {
					log.Printf("route %s: %v", p.AssignedIP, err)
				} else {
					routed[p.AssignedIP] = true
					log.Printf("route added: %s -> %s", p.AssignedIP, ad.Name)
				}
			}
		}
		if sig := peerSig(wgPeers); sig != lastSig {
			if err := dev.SetPeers(wgPeers); err != nil {
				log.Printf("set peers: %v", err)
				return
			}
			lastSig = sig
			curPeers = wgPeers
			log.Printf("applied %d peer(s)", len(wgPeers))
		}
		logHandshakes(dev, curPeers)

		svcs, serr := cl.Services()
		if serr != nil {
			log.Printf("service refresh: %v", serr)
		}
		updateDNS(rslv, suffix, peers, svcs)
		manageHostProxies(hostProxies, selfIP, svcs)
	}

	apply()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			apply()
		}
	}
}

// peerSig is a stable signature of a peer set, used to avoid re-applying an
// unchanged configuration (which would reset in-flight handshakes).
func peerSig(peers []wg.Peer) string {
	parts := make([]string, 0, len(peers))
	for _, p := range peers {
		parts = append(parts, p.PublicKeyHex+"|"+p.Endpoint+"|"+p.AllowedIP)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// updateDNS refreshes the local resolver's records from peers (node hostnames)
// and services, all under the network's DNS suffix.
func updateDNS(rslv *resolver.Resolver, suffix string, peers []proto.Peer, svcs []proto.Service) {
	if rslv == nil {
		return
	}
	recs := map[string]net.IP{}
	for _, p := range peers {
		if p.Hostname == "" {
			continue
		}
		if ip := net.ParseIP(p.AssignedIP); ip != nil {
			recs[strings.ToLower(p.Hostname)+"."+suffix] = ip
		}
	}
	for _, s := range svcs {
		if ip := net.ParseIP(s.NodeIP); ip != nil {
			recs[strings.ToLower(s.Name)+"."+suffix] = ip
		}
	}
	rslv.SetRecords(recs)
}

// manageHostProxies starts an L4 proxy for each service hosted on this node that
// has a localhost backend, so the real backend stays bound to 127.0.0.1 while the
// service is reachable at <selfOverlayIP>:<port> over the overlay.
func manageHostProxies(running map[string]*l4.TCPProxy, selfIP string, svcs []proto.Service) {
	for _, s := range svcs {
		if s.NodeIP != selfIP || s.BackendPort == 0 {
			continue
		}
		if strings.ToLower(s.Proto) != "tcp" {
			continue // UDP proxy not implemented yet
		}
		if _, ok := running[s.Name]; ok {
			continue
		}
		listen := fmt.Sprintf("%s:%d", selfIP, s.Port)
		backend := fmt.Sprintf("127.0.0.1:%d", s.BackendPort)
		p, err := l4.ListenTCP(listen, backend)
		if err != nil {
			log.Printf("service %s proxy on %s: %v", s.Name, listen, err)
			continue
		}
		running[s.Name] = p
		log.Printf("serving %s on %s -> %s", s.Name, listen, backend)
	}
}

// logHandshakes reports each peer's WireGuard handshake state. A non-zero
// handshake proves the encrypted tunnel to that peer is established.
func logHandshakes(dev *wg.Device, peers []wg.Peer) {
	hs, err := dev.PeerHandshakes()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	for _, p := range peers {
		if t := hs[p.PublicKeyHex]; t > 0 {
			log.Printf("  tunnel %s: handshake OK (%ds ago)", p.AllowedIP, now-t)
		} else {
			log.Printf("  tunnel %s: awaiting handshake ...", p.AllowedIP)
		}
	}
}

func closeAdapter(ad *tun.Adapter, name string) {
	log.Printf("removing adapter %q", name)
	_ = ad.Close()
}

func waitForInterrupt() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

// trustLabel describes how the control server's identity was verified.
func trustLabel(cl *join.Client) string {
	switch {
	case strings.HasPrefix(cl.BaseURL(), "http://"):
		return "plaintext - no server authentication"
	case cl.Pin() != "":
		return "TLS, pinned " + cl.Pin()
	default:
		return "TLS, CA-verified"
	}
}
