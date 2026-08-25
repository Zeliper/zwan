// Package manager supervises the networks this device has joined.
//
// A device can be a member of several networks at once (design docs 37-40), so
// the parts that are naturally singular have to be shared or separated
// deliberately:
//
//   - one resolver for every network, because 127.0.0.1:53 can only be owned
//     once; each network gets a zone inside it, keyed by its local alias;
//   - one Wintun adapter, node key and UDP port per network, so nothing is
//     shared that could leak one network's traffic into another;
//   - names are scoped by the device's own alias for a network, since two
//     servers may advertise the same DNS suffix.
//
// Isolation between networks falls out of that separation: each engine only
// programs its own adapter with its own peers, and nothing routes between them
// unless the operating system is told to forward, which it is not.
package manager

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/client/resolver"
)

// DefaultBasePort is the first WireGuard UDP port tried for a network.
const DefaultBasePort = 51820

// Config configures the manager.
type Config struct {
	DNSAddr     string // resolver listen address ("" disables local DNS)
	ProductName string
	DeviceUUID  string
	BasePort    int // first WireGuard UDP port to try (default DefaultBasePort)
}

// Status is one network's saved settings plus its live state.
type Status struct {
	Network profile.Network `json:"network"`
	Engine  engine.Status   `json:"engine"`

	// Warning describes a problem that is not an error: the network is up but
	// something about the combination needs the operator's attention.
	Warning string `json:"warning,omitempty"`
}

type entry struct {
	eng  *engine.Engine
	port int
}

// Manager owns every joined network on this device.
type Manager struct {
	cfg Config

	mu    sync.Mutex
	dns   *resolver.Resolver
	nets  map[string]*entry
	saved []profile.Network
}

// New returns an idle manager.
func New(cfg Config) *Manager {
	if cfg.BasePort == 0 {
		cfg.BasePort = DefaultBasePort
	}
	if cfg.ProductName == "" {
		cfg.ProductName = "zwan"
	}
	return &Manager{cfg: cfg, nets: map[string]*entry{}}
}

// Start brings up the shared resolver and reconnects the networks the device
// was left connected to.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.cfg.DNSAddr != "" && m.dns == nil {
		r := resolver.New()
		if _, err := r.Listen(m.cfg.DNSAddr); err != nil {
			log.Printf("manager: dns resolver on %s: %v (continuing without local DNS)", m.cfg.DNSAddr, err)
		} else {
			go func() { _ = r.Serve() }()
			m.dns = r
		}
	}
	saved, err := profile.LoadNetworks()
	if err != nil {
		log.Printf("manager: load networks: %v", err)
	}
	m.saved = saved
	m.mu.Unlock()

	for _, n := range saved {
		if !n.AutoConnect {
			continue
		}
		if err := m.Connect(n); err != nil {
			log.Printf("manager: reconnect %s: %v", n.Alias, err)
		}
	}
}

// Connect joins a network and remembers it. Reconnecting an alias that is
// already up replaces it, so editing a network's settings is one call.
func (m *Manager) Connect(n profile.Network) error {
	n.Alias = profile.NormalizeAlias(n.Alias)
	if err := n.Validate(); err != nil {
		return err
	}
	n.AutoConnect = true

	m.mu.Lock()
	if prev, ok := m.nets[n.Alias]; ok {
		delete(m.nets, n.Alias)
		m.mu.Unlock()
		prev.eng.Stop()
		m.mu.Lock()
	}
	port := m.freePortLocked()
	dns := m.dns
	cfg := m.cfg
	m.mu.Unlock()

	eng := engine.New()
	err := eng.Start(engine.Config{
		Server:      n.Server,
		Pin:         n.Pin,
		Token:       n.Token,
		DeviceUUID:  cfg.DeviceUUID,
		Name:        n.Name,
		UseRelay:    n.UseRelay,
		AdapterName: fmt.Sprintf("%s-%s", cfg.ProductName, n.Alias),
		WGPort:      port,
		Alias:       n.Alias,
		DNS:         zoneOf(dns),
		ProductName: cfg.ProductName,
	})
	if err != nil {
		// Remember it anyway: a server that is down should not lose the
		// network, and the next Start will try again.
		m.remember(n)
		return err
	}

	m.mu.Lock()
	m.nets[n.Alias] = &entry{eng: eng, port: port}
	m.mu.Unlock()
	m.remember(n)
	return nil
}

// Disconnect takes a network down but keeps it in the list.
func (m *Manager) Disconnect(alias string) error {
	alias = profile.NormalizeAlias(alias)
	m.mu.Lock()
	e, ok := m.nets[alias]
	delete(m.nets, alias)
	m.mu.Unlock()
	if ok {
		e.eng.Stop()
	}

	m.mu.Lock()
	for i := range m.saved {
		if m.saved[i].Alias == alias {
			m.saved[i].AutoConnect = false
		}
	}
	nets := append([]profile.Network(nil), m.saved...)
	m.mu.Unlock()
	return profile.SaveNetworks(nets)
}

// Forget disconnects a network and removes it from the device.
func (m *Manager) Forget(alias string) error {
	alias = profile.NormalizeAlias(alias)
	_ = m.Disconnect(alias)

	m.mu.Lock()
	kept := m.saved[:0]
	for _, n := range m.saved {
		if n.Alias != alias {
			kept = append(kept, n)
		}
	}
	m.saved = append([]profile.Network(nil), kept...)
	nets := append([]profile.Network(nil), m.saved...)
	m.mu.Unlock()
	return profile.SaveNetworks(nets)
}

// Statuses reports every known network, connected or not, ordered by alias.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	saved := append([]profile.Network(nil), m.saved...)
	live := make(map[string]*entry, len(m.nets))
	for k, v := range m.nets {
		live[k] = v
	}
	m.mu.Unlock()

	out := make([]Status, 0, len(saved))
	for _, n := range saved {
		st := Status{Network: n}
		if e, ok := live[n.Alias]; ok {
			st.Engine = e.eng.Status()
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Network.Alias < out[j].Network.Alias })
	addOverlapWarnings(out)
	return out
}

// Stop takes every network down and releases the resolver.
func (m *Manager) Stop() {
	m.mu.Lock()
	engines := make([]*engine.Engine, 0, len(m.nets))
	for _, e := range m.nets {
		engines = append(engines, e.eng)
	}
	m.nets = map[string]*entry{}
	dns := m.dns
	m.dns = nil
	m.mu.Unlock()

	for _, e := range engines {
		e.Stop()
	}
	if dns != nil {
		_ = dns.Shutdown()
	}
}

// remember stores or replaces a network in the saved list.
func (m *Manager) remember(n profile.Network) {
	m.mu.Lock()
	replaced := false
	for i := range m.saved {
		if m.saved[i].Alias == n.Alias {
			m.saved[i] = n
			replaced = true
			break
		}
	}
	if !replaced {
		m.saved = append(m.saved, n)
	}
	nets := append([]profile.Network(nil), m.saved...)
	m.mu.Unlock()
	if err := profile.SaveNetworks(nets); err != nil {
		log.Printf("manager: save networks: %v", err)
	}
}

// freePortLocked picks a UDP port no other network on this device is using.
// Callers hold the mutex.
func (m *Manager) freePortLocked() int {
	taken := make(map[int]bool, len(m.nets))
	for _, e := range m.nets {
		taken[e.port] = true
	}
	for port := m.cfg.BasePort; port < m.cfg.BasePort+64; port++ {
		if taken[port] || !udpFree(port) {
			continue
		}
		return port
	}
	return 0 // let the OS choose; direct mode then needs an explicit endpoint
}

func udpFree(port int) bool {
	pc, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = pc.Close()
	return true
}

// addOverlapWarnings flags networks whose overlay ranges collide.
//
// Until addresses are translated per network (design doc 38), two networks using
// the same range can hand out the same address, and the host has one routing
// table to put it in. Saying so is better than a peer silently going missing.
func addOverlapWarnings(list []Status) {
	type span struct {
		alias  string
		prefix netip.Prefix
	}
	var spans []span
	for _, s := range list {
		if !s.Engine.Connected || s.Engine.OverlayCIDR == "" {
			continue
		}
		p, err := netip.ParsePrefix(s.Engine.OverlayCIDR)
		if err != nil {
			continue
		}
		spans = append(spans, span{alias: s.Network.Alias, prefix: p})
	}
	clash := map[string][]string{}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].prefix.Overlaps(spans[j].prefix) {
				clash[spans[i].alias] = append(clash[spans[i].alias], spans[j].alias)
				clash[spans[j].alias] = append(clash[spans[j].alias], spans[i].alias)
			}
		}
	}
	for i := range list {
		others := clash[list[i].Network.Alias]
		if len(others) == 0 {
			continue
		}
		sort.Strings(others)
		list[i].Warning = fmt.Sprintf(
			"overlay range %s overlaps %s; addresses can collide until per-network translation lands",
			list[i].Engine.OverlayCIDR, strings.Join(others, ", "))
	}
}

// zoneOf adapts the resolver to the engine's sink, keeping a nil resolver nil
// rather than handing the engine a non-nil interface wrapping nothing.
func zoneOf(r *resolver.Resolver) engine.Zone {
	if r == nil {
		return nil
	}
	return r
}
