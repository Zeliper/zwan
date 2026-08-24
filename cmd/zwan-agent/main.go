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
	"os"
	"os/signal"
	"time"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:8787", "control server URL")
	token := flag.String("token", "", "join token")
	device := flag.String("device", "", "stable device UUID")
	name := flag.String("name", "", "hostname label (defaults to OS hostname)")
	useTun := flag.Bool("tun", false, "create the adapter and assign the overlay IP (Administrator)")
	useUp := flag.Bool("up", false, "create the adapter and start the WireGuard tunnel to peers (Administrator)")
	adapter := flag.String("adapter", "", "adapter name (default \"<Product>-<network>\")")
	wgPort := flag.Int("wg-port", 51820, "local WireGuard UDP listen port")
	endpoint := flag.String("endpoint", "", "endpoint host:port peers use to reach us (default 127.0.0.1:<wg-port>)")
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

	res, err := join.Do(*server, *token, *device, hostname, ep)
	if err != nil {
		log.Fatalf("join failed: %v", err)
	}
	log.Printf("joined: network=%s suffix=%s assigned_ip=%s",
		res.Register.NetworkID, res.Register.DNSSuffix, res.Register.AssignedIP)
	log.Printf("node public key: %s", res.PublicKey)
	printPeers(res.Peers)

	adapterName := *adapter
	if adapterName == "" {
		adapterName = fmt.Sprintf("%s-%s", shared.ProductName, res.Register.NetworkID)
	}

	switch {
	case *useUp:
		runTunnel(res, *server, adapterName, *wgPort)
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

// runTunnel (M1b-2): adapter + wireguard-go device + periodic peer refresh.
func runTunnel(res *join.Result, serverURL, adapterName string, wgPort int) {
	log.Printf("creating adapter %q ...", adapterName)
	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		log.Fatalf("adapter: %v (Administrator? wintun.dll present?)", err)
	}
	defer closeAdapter(ad, adapterName)

	if err := ad.SetNodeIP(res.Register.AssignedIP); err != nil {
		log.Fatalf("assign overlay IP: %v", err)
	}
	dev, err := wg.Up(ad, res.Private, wgPort)
	if err != nil {
		log.Fatalf("wireguard: %v", err)
	}
	defer dev.Close()

	log.Printf("tunnel up on %q (%s), wg-port %d. Ctrl+C to stop.", adapterName, res.Register.AssignedIP, wgPort)

	stop := make(chan struct{})
	go refreshPeers(dev, ad, serverURL, res.Register.AssignedIP, stop)
	waitForInterrupt()
	close(stop)
}

// refreshPeers polls the control server and reprograms the tunnel + routes as
// peers appear or change.
func refreshPeers(dev *wg.Device, ad *tun.Adapter, serverURL, selfIP string, stop <-chan struct{}) {
	routed := map[string]bool{}
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	apply := func() {
		peers, err := join.FetchPeers(serverURL)
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
			wgPeers = append(wgPeers, wg.Peer{
				PublicKeyHex: hexKey,
				Endpoint:     p.Endpoint,
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
		if err := dev.SetPeers(wgPeers); err != nil {
			log.Printf("set peers: %v", err)
			return
		}
		logHandshakes(dev, wgPeers)
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
