// Package ipam allocates overlay IP addresses from a network's CIDR.
//
// M1a: in-memory, stable per device key. A device that re-registers keeps its
// address (matches the "fixed IP by Device UUID" behaviour from the design doc).
package ipam

import (
	"fmt"
	"net/netip"
	"sync"
)

// Allocator hands out addresses from a prefix, stable per key.
type Allocator struct {
	mu     sync.Mutex
	prefix netip.Prefix
	next   netip.Addr
	byKey  map[string]netip.Addr
	used   map[netip.Addr]bool
}

// New creates an allocator over the given CIDR (e.g. "100.64.0.0/16").
// Allocation starts at the first host address (network address is skipped).
func New(cidr string) (*Allocator, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	p = p.Masked()
	return &Allocator{
		prefix: p,
		next:   p.Addr().Next(),
		byKey:  map[string]netip.Addr{},
		used:   map[netip.Addr]bool{},
	}, nil
}

// Allocate returns the address for key, assigning a new one on first use.
func (a *Allocator) Allocate(key string) (netip.Addr, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ip, ok := a.byKey[key]; ok {
		return ip, nil
	}
	for a.prefix.Contains(a.next) {
		ip := a.next
		a.next = a.next.Next()
		if a.used[ip] {
			continue
		}
		a.used[ip] = true
		a.byKey[key] = ip
		return ip, nil
	}
	return netip.Addr{}, fmt.Errorf("ipam: address space %s exhausted", a.prefix)
}
