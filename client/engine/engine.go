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
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/client/wgbind"
	"github.com/Zeliper/zwan/shared/keys"
	"github.com/Zeliper/zwan/shared/proto"
)

// Config controls a single connection.
type Config struct {
	Server      string // control server base URL
	Token       string
	DeviceUUID  string
	Name        string
	UseRelay    bool
	AdapterName string // default "<Product>-<network>"
	WGPort      int    // direct-mode UDP listen port (default 51820)
	DNSAddr     string // local resolver address ("" disables it)
	ProductName string // for the default adapter name
}

// Status is a snapshot of the connection for the UI/IPC.
type Status struct {
	Connected   bool            `json:"connected"`
	NetworkID   string          `json:"networkId"`
	DNSSuffix   string          `json:"dnsSuffix"`
	OverlayCIDR string          `json:"overlayCidr"`
	AssignedIP  string          `json:"assignedIp"`
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

	endpoint := fmt.Sprintf("127.0.0.1:%d", cfg.WGPort)
	res, err := join.Do(cfg.Server, cfg.Token, cfg.DeviceUUID, cfg.Name, endpoint)
	if err != nil {
		e.setErr("join: %v", err)
		return err
	}

	adapterName := cfg.AdapterName
	if adapterName == "" {
		adapterName = fmt.Sprintf("%s-%s", cfg.ProductName, res.Register.NetworkID)
	}

	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		e.setErr("adapter: %v", err)
		return err
	}
	if err := ad.SetNodeIP(res.Register.AssignedIP); err != nil {
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

	var rslv *resolver.Resolver
	if cfg.DNSAddr != "" {
		rslv = resolver.New(res.Register.DNSSuffix)
		if _, err := rslv.Listen(cfg.DNSAddr); err != nil {
			log.Printf("engine: dns resolver: %v (continuing)", err)
			rslv = nil
		} else {
			go func() { _ = rslv.Serve() }()
		}
	}

	e.mu.Lock()
	e.stop = make(chan struct{})
	e.done = make(chan struct{})
	e.st = Status{
		Connected: true, NetworkID: res.Register.NetworkID, DNSSuffix: res.Register.DNSSuffix,
		OverlayCIDR: res.Register.OverlayCIDR, AssignedIP: res.Register.AssignedIP,
		RelayAddr: res.Register.RelayAddr, PublicKey: res.PublicKey, Via: via,
		Handshakes: map[string]bool{},
	}
	stop := e.stop
	done := e.done
	e.mu.Unlock()

	go e.run(dev, ad, rslv, res.Register.AssignedIP, res.Register.DNSSuffix, cfg, stop, done)
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

func (e *Engine) run(dev *wg.Device, ad *tun.Adapter, rslv *resolver.Resolver, selfIP, suffix string, cfg Config, stop, done chan struct{}) {
	defer close(done)
	defer func() {
		if rslv != nil {
			_ = rslv.Shutdown()
		}
		dev.Close()
		_ = ad.Close()
	}()

	routed := map[string]bool{}
	hostProxies := map[string]*l4.TCPProxy{}
	var lastSig string
	var curPeers []wg.Peer

	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	apply := func() {
		peers, err := join.FetchPeers(cfg.Server)
		if err != nil {
			log.Printf("engine: peer refresh: %v", err)
			return
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
			wgPeers = append(wgPeers, wg.Peer{PublicKeyHex: hexKey, Endpoint: endpoint, AllowedIP: p.AssignedIP + "/32"})
			if !routed[p.AssignedIP] {
				if err := ad.AddPeerRoute(p.AssignedIP); err == nil {
					routed[p.AssignedIP] = true
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

		svcs, _ := join.FetchServices(cfg.Server)
		updateDNS(rslv, suffix, peers, svcs)
		manageHostProxies(hostProxies, selfIP, svcs)

		hs := handshakeMap(dev, curPeers)
		e.mu.Lock()
		e.st.Peers = peers
		e.st.Services = svcs
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

func peerSig(peers []wg.Peer) string {
	parts := make([]string, 0, len(peers))
	for _, p := range peers {
		parts = append(parts, p.PublicKeyHex+"|"+p.Endpoint+"|"+p.AllowedIP)
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
		ip := strings.TrimSuffix(p.AllowedIP, "/32")
		out[ip] = hs[p.PublicKeyHex] > 0
	}
	return out
}

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

func manageHostProxies(running map[string]*l4.TCPProxy, selfIP string, svcs []proto.Service) {
	for _, s := range svcs {
		if s.NodeIP != selfIP || s.BackendPort == 0 || strings.ToLower(s.Proto) != "tcp" {
			continue
		}
		if _, ok := running[s.Name]; ok {
			continue
		}
		listen := fmt.Sprintf("%s:%d", selfIP, s.Port)
		backend := fmt.Sprintf("127.0.0.1:%d", s.BackendPort)
		p, err := l4.ListenTCP(listen, backend)
		if err != nil {
			log.Printf("engine: service %s proxy on %s: %v", s.Name, listen, err)
			continue
		}
		running[s.Name] = p
		log.Printf("engine: serving %s on %s -> %s", s.Name, listen, backend)
	}
}
