// Package resolver is the client-side split-DNS resolver. It answers A queries
// for names under a network's DNS suffix (services and node hostnames) from a
// record table the agent keeps in sync with the control server. Names outside
// the suffix are never sent here (NRPT routes only the suffix to this resolver).
package resolver

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// Resolver is a small authoritative DNS server for one network's suffix.
type Resolver struct {
	suffix string
	ttl    uint32

	mu  sync.RWMutex
	a   map[string]net.IP // fqdn (lowercased, no trailing dot) -> IPv4
	srv *dns.Server
}

// New creates a resolver for suffix (e.g. "demo.zwan").
func New(suffix string) *Resolver {
	return &Resolver{
		suffix: strings.ToLower(strings.Trim(suffix, ".")),
		ttl:    30,
		a:      map[string]net.IP{},
	}
}

// Suffix returns the network suffix this resolver is authoritative for.
func (r *Resolver) Suffix() string { return r.suffix }

// SetRecords replaces the A record table. Keys are full names such as
// "minecraft.demo.zwan"; values are IPv4 addresses.
func (r *Resolver) SetRecords(recs map[string]net.IP) {
	m := make(map[string]net.IP, len(recs))
	for k, v := range recs {
		m[strings.ToLower(strings.TrimSuffix(k, "."))] = v
	}
	r.mu.Lock()
	r.a = m
	r.mu.Unlock()
}

func (r *Resolver) handle(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true

	for _, q := range req.Question {
		if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(q.Name, "."))
		r.mu.RLock()
		ip, ok := r.a[name]
		r.mu.RUnlock()
		if !ok {
			m.Rcode = dns.RcodeNameError // NXDOMAIN within our suffix
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
	mux.HandleFunc(r.suffix+".", r.handle)
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
