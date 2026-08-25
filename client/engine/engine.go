// Package engine runs and supervises a single overlay-network connection:
// control-plane join, the Wintun adapter, the wireguard-go tunnel (direct or
// relay), peer/route sync, the split-DNS resolver and hosted L4 service proxies.
//
// It is the reusable core shared by the CLI agent and the Windows service.
package engine

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/l4"
	"github.com/Zeliper/zwan/client/resolver"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/vip"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/client/wgbind"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

// Zone is where an engine publishes its DNS records.
//
// The manager hands every engine the same resolver, because a device can only
// own 127.0.0.1:53 once; a single-network caller can leave it nil and let the
// engine run a resolver of its own.
type Zone interface {
	SetZone(suffix string, recs map[string]net.IP)
	RemoveZone(suffix string)
}

// Config controls a single connection.
type Config struct {
	Server      string // control server base URL (may carry "#<pin>")
	Pin         string // server key pin; overrides one embedded in Server
	Token       string
	DeviceUUID  string
	Name        string
	UseRelay    bool
	AdapterName string // default "<Product>-<network>"
	WGPort      int    // direct-mode UDP listen port (default 51820)
	Endpoint    string // host:port peers reach us at (default 127.0.0.1:<WGPort>)
	DNSAddr     string // address for an engine-owned resolver ("" disables it; ignored when DNS is set)
	DNS         Zone   // shared record sink; when nil the engine may run its own resolver
	ProductName string // for the default adapter name

	// LocalPrefix gives this network an address range that is unique on this
	// device, so two networks may share an overlay range (design doc 38).
	// Empty means no translation: the device uses the overlay addresses as-is.
	LocalPrefix string

	// Alias is the local DNS suffix for this network. Two networks can carry
	// the same server-side suffix, so the device picks its own short name and
	// reaches services as <service>.<alias> (design doc 47.1). Empty means use
	// the suffix the server advertised.
	Alias string

	// Publish are services this node hosts. The engine keeps them registered:
	// the control server's registry is in memory, so a server restart drops
	// them and the next refresh puts them back.
	Publish []proto.Service
}

// Status is a snapshot of the connection for the UI/IPC.
type Status struct {
	Connected   bool            `json:"connected"`
	Server      string          `json:"server"` // normalized control server URL
	Pinned      bool            `json:"pinned"` // server verified by key pin rather than a CA
	NetworkID   string          `json:"networkId"`
	DNSSuffix   string          `json:"dnsSuffix"`
	OverlayCIDR string          `json:"overlayCidr"`
	AssignedIP  string          `json:"assignedIp"` // the address this device actually holds
	OverlayIP   string          `json:"overlayIp"`  // the address the control server assigned
	LocalCIDR   string          `json:"localCidr"`  // local range in use ("" when not translating)
	RelayAddr   string          `json:"relayAddr"`
	PublicKey   string          `json:"publicKey"`
	Via         string          `json:"via"` // "relay" or "direct"
	Peers       []proto.Peer    `json:"peers"`
	Services    []proto.Service `json:"services"`
	Handshakes  map[string]bool `json:"handshakes"` // peer IP -> tunnel established
	LastError   string          `json:"lastError"`
}

// Engine supervises one connection. Zero value is not usable; use New.
type Engine struct {
	mu   sync.Mutex
	cfg  Config
	stop chan struct{}
	done chan struct{}
	st   Status
}

// New returns an idle engine.
func New() *Engine { return &Engine{} }

// Status returns the current snapshot.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

func (e *Engine) setErr(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	e.mu.Lock()
	e.st.LastError = msg
	e.mu.Unlock()
	log.Print(msg)
}

// Start brings the connection up. It performs the control-plane join
// synchronously (so errors are returned) and then runs the data plane in the
// background until Stop.
func (e *Engine) Start(cfg Config) error {
	e.mu.Lock()
	if e.stop != nil {
		e.mu.Unlock()
		return fmt.Errorf("engine already running")
	}
	if cfg.WGPort == 0 {
		cfg.WGPort = 51820
	}
	if cfg.ProductName == "" {
		cfg.ProductName = "zwan"
	}
	e.cfg = cfg
	e.st = Status{}
	e.mu.Unlock()

	cl, err := join.NewClient(cfg.Server, cfg.Pin)
	if err != nil {
		e.setErr("control server: %v", err)
		return err
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("127.0.0.1:%d", cfg.WGPort)
	}
	res, err := cl.Join(cfg.Token, cfg.DeviceUUID, cfg.Name, endpoint)
	if err != nil {
		e.setErr("join: %v", err)
		return err
	}

	adapterName := cfg.AdapterName
	if adapterName == "" {
		adapterName = fmt.Sprintf("%s-%s", cfg.ProductName, res.Register.NetworkID)
	}

	xlate, err := newTranslator(cfg.LocalPrefix, res.Register.OverlayCIDR)
	if err != nil {
		e.setErr("local address range: %v", err)
		return err
	}

	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		e.setErr("adapter: %v", err)
		return err
	}
	// With translation on, the adapter carries this device's local address and
	// the tunnel carries the overlay one; the wrapper sits between them.
	deviceIP := res.Register.AssignedIP
	if xlate.on() {
		selfAddr, err := netip.ParseAddr(res.Register.AssignedIP)
		if err != nil {
			_ = ad.Close()
			e.setErr("assigned ip: %v", err)
			return err
		}
		wrapped, local, err := vip.Wrap(ad.Dev, xlate.table, selfAddr)
		if err != nil {
			_ = ad.Close()
			e.setErr("local address: %v", err)
			return err
		}
		ad.Dev = wrapped
		deviceIP = local.String()
	}
	if err := ad.SetNodeIP(deviceIP); err != nil {
		_ = ad.Close()
		e.setErr("assign ip: %v", err)
		return err
	}

	var dev *wg.Device
	via := "direct"
	if cfg.UseRelay {
		if res.Register.RelayAddr == "" {
			_ = ad.Close()
			return fmt.Errorf("relay requested but server advertised none")
		}
		selfIP, err := netip.ParseAddr(res.Register.AssignedIP)
		if err != nil {
			_ = ad.Close()
			return err
		}
		bind, err := wgbind.NewRelay(res.Register.RelayAddr, selfIP)
		if err != nil {
			_ = ad.Close()
			return err
		}
		dev, err = wg.Up(ad, res.Private, cfg.WGPort, bind)
		via = "relay"
		if err != nil {
			_ = ad.Close()
			e.setErr("wireguard: %v", err)
			return err
		}
	} else {
		dev, err = wg.Up(ad, res.Private, cfg.WGPort, wgbind.New())
		if err != nil {
			_ = ad.Close()
			e.setErr("wireguard: %v", err)
			return err
		}
	}

	suffix := strings.ToLower(strings.Trim(strings.TrimSpace(cfg.Alias), "."))
	if suffix == "" {
		suffix = res.Register.DNSSuffix
	}

	// A shared resolver wins; otherwise run one for this network alone.
	zone := cfg.DNS
	var owned *resolver.Resolver
	if zone == nil && cfg.DNSAddr != "" {
		owned = resolver.New()
		if _, err := owned.Listen(cfg.DNSAddr); err != nil {
			log.Printf("engine: dns resolver: %v (continuing)", err)
			owned = nil
		} else {
			go func() { _ = owned.Serve() }()
			zone = owned
		}
	}

	e.mu.Lock()
	e.stop = make(chan struct{})
	e.done = make(chan struct{})
	e.st = Status{
		Connected: true, Server: cl.BaseURL(), Pinned: cl.Pin() != "",
		NetworkID: res.Register.NetworkID, DNSSuffix: suffix,
		OverlayCIDR: res.Register.OverlayCIDR, AssignedIP: deviceIP,
		OverlayIP: res.Register.AssignedIP, LocalCIDR: xlate.cidr(),
		RelayAddr: res.Register.RelayAddr, PublicKey: res.PublicKey, Via: via,
		Handshakes: map[string]bool{},
	}
	stop := e.stop
	done := e.done
	e.mu.Unlock()

	go e.run(cl, dev, ad, zone, owned, xlate, res.Register.AssignedIP, deviceIP, suffix, cfg, stop, done)
	return nil
}

// Stop tears the connection down and blocks until cleanup completes.
func (e *Engine) Stop() {
	e.mu.Lock()
	stop, done := e.stop, e.done
	e.stop, e.done = nil, nil
	e.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
	e.mu.Lock()
	e.st.Connected = false
	e.mu.Unlock()
}

func (e *Engine) run(cl *join.Client, dev *wg.Device, ad *tun.Adapter, zone Zone, owned *resolver.Resolver, xlate translator, selfIP, deviceIP, suffix string, cfg Config, stop, done chan struct{}) {
	defer close(done)
	defer func() {
		// Drop this network's names first: leaving a network must stop its
		// names resolving, even when the resolver is shared and keeps running.
		if zone != nil {
			zone.RemoveZone(suffix)
		}
		if owned != nil {
			_ = owned.Shutdown()
		}
		dev.Close()
		_ = ad.Close()
	}()

	routed := map[string]bool{}
	hostProxies := map[string]*hostedService{}
	access := &l4.AccessPolicy{}
	var lastSig string
	var curPeers []wg.Peer

	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	apply := func() {
		peers, err := cl.Peers()
		if err != nil {
			log.Printf("engine: peer refresh: %v", err)
			return
		}
		svcs, _ := cl.Services()
		svcs = republish(cl, cfg.Publish, svcs, selfIP)

		// A service has an address of its own, and that address lives on the
		// node hosting it — so the tunnel to that node has to be willing to
		// carry it, or packets for the service match no peer and go nowhere.
		hosted := map[string][]string{}
		for _, sv := range svcs {
			if sv.VIP != "" {
				hosted[sv.NodeIP] = append(hosted[sv.NodeIP], sv.VIP)
			}
		}

		var wgPeers []wg.Peer
		for _, p := range peers {
			if p.AssignedIP == selfIP || p.PublicKey == "" {
				continue
			}
			hexKey, err := keys.PublicHexFromBase64(p.PublicKey)
			if err != nil {
				continue
			}
			endpoint := p.Endpoint
			if cfg.UseRelay {
				endpoint = p.AssignedIP
			}
			// WireGuard is addressed in overlay space; the operating system is
			// addressed in local space.
			allowed := append([]string{p.AssignedIP + "/32"}, cidrs(hosted[p.AssignedIP])...)
			wgPeers = append(wgPeers, wg.Peer{PublicKeyHex: hexKey, Endpoint: endpoint, AllowedIPs: allowed})
			local, ok := xlate.local(p.AssignedIP)
			if !ok {
				continue
			}
			if !routed[local] {
				if err := ad.AddPeerRoute(local); err == nil {
					routed[local] = true
				}
			}
		}
		if sig := peerSig(wgPeers); sig != lastSig {
			if err := dev.SetPeers(wgPeers); err != nil {
				log.Printf("engine: set peers: %v", err)
				return
			}
			lastSig, curPeers = sig, wgPeers
		}

		// Everything above this line speaks overlay addresses; everything below
		// speaks the addresses this device actually uses.
		shownPeers := xlate.peers(peers)
		shownSvcs := xlate.services(svcs)
		routeServices(ad, routed, deviceIP, shownSvcs)
		updateDNS(zone, suffix, shownPeers, shownSvcs)
		access.Set(shownPeers, shownSvcs)
		manageHostProxies(hostProxies, ad, deviceIP, shownSvcs, access)

		hs := xlate.handshakes(handshakeMap(dev, curPeers))
		e.mu.Lock()
		e.st.Peers = shownPeers
		e.st.Services = shownSvcs
		e.st.Handshakes = hs
		e.mu.Unlock()
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

// republish re-registers any service this node hosts that the server does not
// currently list, and returns the list including them. The registry lives in the
// server's memory, so this is what makes a hosted service survive a control
// server restart without the operator noticing.
func republish(cl *join.Client, want []proto.Service, have []proto.Service, selfIP string) []proto.Service {
	if len(want) == 0 {
		return have
	}
	listed := make(map[string]bool, len(have))
	for _, s := range have {
		listed[s.Name] = true
	}
	for _, s := range want {
		if listed[s.Name] {
			continue
		}
		s.NodeIP = selfIP
		stored, err := cl.PublishService(s)
		if err != nil {
			log.Printf("engine: publish %s: %v", s.Name, err)
			continue
		}
		log.Printf("engine: published service %s at %s:%d/%s", stored.Name, serviceAddr(stored), stored.Port, stored.Proto)
		have = append(have, stored)
	}
	return have
}

// serviceAddr is where a service answers: its own address when the server gave
// it one, and a port on the hosting node otherwise.
func serviceAddr(s proto.Service) string {
	if s.VIP != "" {
		return s.VIP
	}
	return s.NodeIP
}

func peerSig(peers []wg.Peer) string {
	parts := make([]string, 0, len(peers))
	for _, p := range peers {
		parts = append(parts, p.PublicKeyHex+"|"+p.Endpoint+"|"+strings.Join(p.AllowedIPs, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func handshakeMap(dev *wg.Device, peers []wg.Peer) map[string]bool {
	out := map[string]bool{}
	hs, err := dev.PeerHandshakes()
	if err != nil {
		return out
	}
	for _, p := range peers {
		if len(p.AllowedIPs) == 0 {
			continue
		}
		// The first entry is the peer's own address; the rest are services it
		// hosts, which share the peer's tunnel state.
		ip := strings.TrimSuffix(p.AllowedIPs[0], "/32")
		out[ip] = hs[p.PublicKeyHex] > 0
	}
	return out
}

func updateDNS(zone Zone, suffix string, peers []proto.Peer, svcs []proto.Service) {
	if zone == nil {
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
		// The service's own address is the point of having one: it is what makes
		// the name enough, with no port for the user to know or type.
		target := s.VIP
		if target == "" {
			target = s.NodeIP
		}
		if ip := net.ParseIP(target); ip != nil {
			recs[strings.ToLower(s.Name)+"."+suffix] = ip
		}
	}
	zone.SetZone(suffix, recs)
}

// hostedService is one service this node answers for on the overlay.
type hostedService struct {
	tcp *l4.TCPProxy
	udp *l4.UDPProxy
}

// cidrs turns addresses into /32 prefixes for a peer's allowed list.
func cidrs(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a+"/32")
	}
	return out
}

// routeServices sends each service's address into the tunnel. Services on this
// node are skipped: their addresses sit on the adapter itself.
func routeServices(ad *tun.Adapter, routed map[string]bool, selfIP string, svcs []proto.Service) {
	for _, s := range svcs {
		if s.VIP == "" || s.NodeIP == selfIP || routed[s.VIP] {
			continue
		}
		if err := ad.AddPeerRoute(s.VIP); err == nil {
			routed[s.VIP] = true
		}
	}
}

// manageHostProxies answers for each service hosted on this node.
//
// The service's own address goes on the adapter and the listener binds to it,
// which is what lets two services here use the same port — and lets a client
// reach either by name without being told a port. The listener consults access
// on every connection, so the allow list is enforced here too and not only by
// what the control server chooses to advertise.
func manageHostProxies(running map[string]*hostedService, ad *tun.Adapter, selfIP string, svcs []proto.Service, access *l4.AccessPolicy) {
	for _, s := range svcs {
		if s.NodeIP != selfIP || s.BackendPort == 0 {
			continue
		}
		if _, ok := running[s.Name]; ok {
			continue
		}
		// Without an address of its own — an older server — the service falls
		// back to a port on this node, as it always was.
		addr := s.VIP
		if addr == "" {
			addr = selfIP
		} else if err := ad.AddServiceIP(addr); err != nil {
			log.Printf("engine: service %s address %s: %v", s.Name, addr, err)
			continue
		}
		listen := fmt.Sprintf("%s:%d", addr, s.Port)
		backend := fmt.Sprintf("127.0.0.1:%d", s.BackendPort)

		hosted := &hostedService{}
		var err error
		if strings.EqualFold(s.Proto, "udp") {
			hosted.udp, err = l4.ListenUDP(listen, backend, access.Filter(s.Name))
		} else {
			hosted.tcp, err = l4.ListenTCP(listen, backend, access.Filter(s.Name))
		}
		if err != nil {
			log.Printf("engine: service %s on %s: %v", s.Name, listen, err)
			continue
		}
		running[s.Name] = hosted
		log.Printf("engine: serving %s on %s/%s -> %s", s.Name, listen, strings.ToLower(s.Proto), backend)
	}
}
