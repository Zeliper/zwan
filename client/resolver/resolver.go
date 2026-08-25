// Package resolver is the client-side split-DNS resolver. It answers A queries
// for names under the networks this device has joined — services and node
// hostnames — from a record table the engine keeps in sync with each control
// server.
//
// One resolver serves every joined network. A device can only own 127.0.0.1:53
// once, so multi-network support has to be zones inside a single listener rather
// than a listener per network. Names outside the joined suffixes never arrive
// here: NRPT routes only those suffixes to this resolver.
package resolver

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// Resolver is a small authoritative DNS server for the joined networks.
type Resolver struct {
	ttl uint32

	mu    sync.RWMutex
	zones map[string]map[string]net.IP // suffix -> fqdn (lowercased, no trailing dot) -> IPv4
	srv   *dns.Server
}

// New creates an empty resolver with no zones.
func New() *Resolver {
	return &Resolver{ttl: 30, zones: map[string]map[string]net.IP{}}
}

// SetZone replaces the records for one network's suffix. Keys are full names
// such as "minecraft.home"; values are IPv4 addresses.
func (r *Resolver) SetZone(suffix string, recs map[string]net.IP) {
	suffix = normalize(suffix)
	if suffix == "" {
		return
	}
	m := make(map[string]net.IP, len(recs))
	for k, v := range recs {
		m[normalize(k)] = v
	}
	r.mu.Lock()
	r.zones[suffix] = m
	r.mu.Unlock()
}

// RemoveZone drops a network's records, so its names stop resolving the moment
// the device leaves it.
func (r *Resolver) RemoveZone(suffix string) {
	suffix = normalize(suffix)
	r.mu.Lock()
	delete(r.zones, suffix)
	r.mu.Unlock()
}

// Suffixes lists the zones currently served, sorted. The caller uses it to keep
// the system's NRPT rules in step with what the resolver actually answers.
func (r *Resolver) Suffixes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.zones))
	for s := range r.zones {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// lookup finds a name, and reports whether any joined network owns its suffix.
// The distinction matters: a name inside one of our zones that we do not have is
// NXDOMAIN, while a name outside every zone was misrouted here and is refused
// rather than answered with a guess.
func (r *Resolver) lookup(name string) (ip net.IP, inZone bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for suffix, recs := range r.zones {
		if name != suffix && !strings.HasSuffix(name, "."+suffix) {
			continue
		}
		inZone = true
		if got, ok := recs[name]; ok {
			return got, true
		}
	}
	return nil, inZone
}

func (r *Resolver) handle(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true

	for _, q := range req.Question {
		if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA {
			continue
		}
		ip, inZone := r.lookup(normalize(q.Name))
		if !inZone {
			m.Rcode = dns.RcodeRefused
			continue
		}
		if ip == nil {
			m.Rcode = dns.RcodeNameError // NXDOMAIN within a joined suffix
			continue
		}
		if q.Qtype == dns.TypeA {
			rr, err := dns.NewRR(fmt.Sprintf("%s %d IN A %s", q.Name, r.ttl, ip.String()))
			if err == nil {
				m.Answer = append(m.Answer, rr)
			}
		}
		// AAAA: no IPv6 overlay addresses yet -> empty answer (NOERROR).
	}
	_ = w.WriteMsg(m)
}

// Listen binds the UDP DNS socket on addr and returns the bound address.
func (r *Resolver) Listen(addr string) (string, error) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return "", err
	}
	mux := dns.NewServeMux()
	// Zones come and go as networks are joined and left, so the handler covers
	// the whole tree and sorts it out per query.
	mux.HandleFunc(".", r.handle)
	r.srv = &dns.Server{PacketConn: pc, Handler: mux}
	return pc.LocalAddr().String(), nil
}

// Serve runs the resolver until Shutdown.
func (r *Resolver) Serve() error { return r.srv.ActivateAndServe() }

// Shutdown stops the resolver.
func (r *Resolver) Shutdown() error {
	if r.srv != nil {
		return r.srv.Shutdown()
	}
	return nil
}

func normalize(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), "."))
}
