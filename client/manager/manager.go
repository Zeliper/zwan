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
	"time"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/nrpt"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/client/resolver"
)

// DefaultBasePort is the first WireGuard UDP port tried for a network.
const DefaultBasePort = 51820

// DefaultLocalPool is carved into a range per network so two networks can share
// an overlay range (design doc 38).
//
// It sits in the carrier-grade NAT block, which is where overlay addressing
// lives by convention and where a home or office LAN will not be. Only these
// addresses reach the host's routing table; a network's real range never does,
// so a server is free to use this block as its overlay range too.
const DefaultLocalPool = "100.112.0.0/12"

// DefaultLocalBits is the prefix length each network gets out of the pool.
const DefaultLocalBits = 16

// Config configures the manager.
type Config struct {
	DNSAddr     string // resolver listen address ("" disables local DNS)
	ProductName string
	DeviceUUID  string
	BasePort    int // first WireGuard UDP port to try (default DefaultBasePort)

	// LocalPool is carved into a range per network; LocalBits is how large each
	// slice is. NoTranslate turns the whole scheme off, so every network uses
	// its overlay addresses directly and only one of any overlapping pair works.
	LocalPool   string
	LocalBits   int
	NoTranslate bool
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
	eng    *engine.Engine
	port   int
	prefix netip.Prefix // this network's slice of the local pool
}

// stop tears the network down. An entry can exist without a running engine —
// a network is remembered even when it failed to start — so this is where that
// is handled rather than at every call site.
func (e *entry) stop() {
	if e == nil || e.eng == nil {
		return
	}
	e.eng.Stop()
}

// Manager owns every joined network on this device.
type Manager struct {
	cfg Config

	mu    sync.Mutex
	dns   *resolver.Resolver
	nets  map[string]*entry
	saved []profile.Network

	// bind ties the resolver into the system's name resolution. bindStop and
	// bindDone belong to the goroutine that keeps it in step with the zones the
	// resolver actually answers for.
	bind     *nrpt.Binder
	bindStop chan struct{}
	bindDone chan struct{}
}

// New returns an idle manager.
func New(cfg Config) *Manager {
	if cfg.BasePort == 0 {
		cfg.BasePort = DefaultBasePort
	}
	if cfg.ProductName == "" {
		cfg.ProductName = "zwan"
	}
	if cfg.LocalPool == "" {
		cfg.LocalPool = DefaultLocalPool
	}
	if cfg.LocalBits == 0 {
		cfg.LocalBits = DefaultLocalBits
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
			m.startBindingLocked()
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
	n.Publish = profile.NormalizePublish(n.Publish)
	if err := n.Validate(); err != nil {
		return err
	}
	n.AutoConnect = true

	m.mu.Lock()
	if prev, ok := m.nets[n.Alias]; ok {
		delete(m.nets, n.Alias)
		m.mu.Unlock()
		prev.stop()
		m.mu.Lock()
	}
	port := m.freePortLocked()
	prefix, prefixErr := m.freePrefixLocked()
	dns := m.dns
	cfg := m.cfg
	m.mu.Unlock()
	if prefixErr != nil {
		m.remember(n)
		return prefixErr
	}

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
		LocalPrefix: prefixString(prefix),
		ProductName: cfg.ProductName,
		Publish:     n.Publish,
	})
	if err != nil {
		// Remember it anyway: a server that is down should not lose the
		// network, and the next Start will try again.
		m.remember(n)
		return err
	}

	m.mu.Lock()
	m.nets[n.Alias] = &entry{eng: eng, port: port, prefix: prefix}
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
		e.stop()
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
		if e, ok := live[n.Alias]; ok && e.eng != nil {
			st.Engine = e.eng.Status()
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Network.Alias < out[j].Network.Alias })
	addOverlapWarnings(out)
	return out
}

// Stop takes every network down, unbinds the system's name resolution and
// releases the resolver.
func (m *Manager) Stop() {
	m.mu.Lock()
	stopping := make([]*entry, 0, len(m.nets))
	for _, e := range m.nets {
		stopping = append(stopping, e)
	}
	m.nets = map[string]*entry{}
	dns := m.dns
	m.dns = nil
	bind, bindStop, bindDone := m.bind, m.bindStop, m.bindDone
	m.bind, m.bindStop, m.bindDone = nil, nil, nil
	m.mu.Unlock()

	// The follower goes first: it must not put a rule back after the rules have
	// been taken out.
	if bindStop != nil {
		close(bindStop)
		<-bindDone
	}
	for _, e := range stopping {
		e.stop()
	}
	if dns != nil {
		_ = dns.Shutdown()
	}
	if bind != nil {
		// Policy rules are machine state and outlive this process. Left behind,
		// they would keep sending every name under a joined suffix to a
		// resolver that has stopped listening.
		if err := bind.Clear(); err != nil {
			log.Printf("manager: system DNS cleanup: %v", err)
		}
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

// freePrefixLocked carves the next unused slice out of the local pool. Callers
// hold the mutex.
func (m *Manager) freePrefixLocked() (netip.Prefix, error) {
	if m.cfg.NoTranslate {
		return netip.Prefix{}, nil
	}
	pool, err := netip.ParsePrefix(m.cfg.LocalPool)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("local pool %q: %w", m.cfg.LocalPool, err)
	}
	pool = pool.Masked()
	bits := m.cfg.LocalBits
	if bits <= pool.Bits() || bits > 30 {
		return netip.Prefix{}, fmt.Errorf("local slice /%d does not fit inside pool %s", bits, pool)
	}
	taken := make(map[netip.Prefix]bool, len(m.nets))
	for _, e := range m.nets {
		taken[e.prefix] = true
	}
	step := uint32(1) << uint(32-bits)
	base := prefixBase(pool)
	count := uint32(1) << uint(bits-pool.Bits())
	for i := uint32(0); i < count; i++ {
		p := netip.PrefixFrom(addrFromUint32(base+i*step), bits)
		if !taken[p] {
			return p, nil
		}
	}
	return netip.Prefix{}, fmt.Errorf("local pool %s has no free /%d left", pool, bits)
}

func prefixBase(p netip.Prefix) uint32 {
	b := p.Addr().As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func addrFromUint32(v uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func prefixString(p netip.Prefix) string {
	if !p.IsValid() {
		return ""
	}
	return p.String()
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
		// A translating network keeps its overlay range off the host's routing
		// table, so it cannot collide with anything.
		if !s.Engine.Connected || s.Engine.OverlayCIDR == "" || s.Engine.LocalCIDR != "" {
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
			"overlay range %s overlaps %s, and address translation is off, so one of them will lose peers",
			list[i].Engine.OverlayCIDR, strings.Join(others, ", "))
	}
}

// resyncEvery is how often the system's policy table is re-read even when the
// set of zones has not changed. A group policy refresh or an administrator can
// remove our rules underneath us, and nothing tells us when that happens.
const resyncEvery = 5 * time.Minute

// startBindingLocked hooks the resolver into the system's name resolution, so
// the joined networks' names work for every program on the machine rather than
// only for whoever queries the resolver directly (design doc 39). Callers hold
// the mutex.
func (m *Manager) startBindingLocked() {
	if m.bind != nil {
		return
	}
	if !nrpt.Supported {
		log.Printf("manager: system DNS integration is Windows-only; names resolve through %s only", m.cfg.DNSAddr)
		return
	}
	b, err := nrpt.New(m.cfg.DNSAddr)
	if err != nil {
		log.Printf("manager: system DNS: %v (names resolve through %s only)", err, m.cfg.DNSAddr)
		return
	}
	m.bind = b
	m.bindStop = make(chan struct{})
	m.bindDone = make(chan struct{})
	go m.followZones(b, m.bindStop, m.bindDone)
}

// followZones keeps the system's name resolution policy equal to the zones the
// resolver answers for.
//
// It follows the resolver rather than the list of networks deliberately: a rule
// sends every name under a suffix here and nowhere else, so it must exist only
// while there is an answer to give. Zones appear and disappear inside the
// engines, which is why this polls; the poll itself is a comparison of two
// short string lists, and the operating system is only touched when they differ.
//
// The first pass runs before the first tick, and that is what clears rules left
// behind by a run that was killed rather than stopped.
func (m *Manager) followZones(b *nrpt.Binder, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	resync := time.NewTicker(resyncEvery)
	defer resync.Stop()

	var lastErr string
	for {
		m.mu.Lock()
		dns := m.dns
		m.mu.Unlock()
		var suffixes []string
		if dns != nil {
			suffixes = dns.Suffixes()
		}
		// Report a failure once rather than every two seconds: an operator
		// needs to see it, not to have the log buried in it.
		if err := b.Apply(suffixes); err != nil {
			if msg := err.Error(); msg != lastErr {
				lastErr = msg
				log.Printf("manager: system DNS: %v", err)
			}
		} else {
			lastErr = ""
		}

		select {
		case <-stop:
			return
		case <-resync.C:
			b.Resync()
		case <-tick.C:
		}
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
