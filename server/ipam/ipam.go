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

// Split divides a network's range in half: the lower half addresses nodes, the
// upper half addresses services.
//
// A service having an address of its own is what lets its name identify it
// completely (design docs 12, 16 and 38). Two services can then both sit on the
// port their protocol normally uses, and a client reaches one by name without
// being told a port at all.
func Split(cidr string) (nodes, services *Allocator, err error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, nil, err
	}
	p = p.Masked()
	if !p.Addr().Is4() {
		return nil, nil, fmt.Errorf("ipam: %s must be IPv4", p)
	}
	if p.Bits() > 28 {
		return nil, nil, fmt.Errorf("ipam: %s is too small to hold both nodes and services", p)
	}
	half := p.Bits() + 1
	lower := netip.PrefixFrom(p.Addr(), half).Masked()

	step := uint32(1) << uint(32-half)
	b := p.Addr().As4()
	base := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	top := base + step
	upperAddr := netip.AddrFrom4([4]byte{byte(top >> 24), byte(top >> 16), byte(top >> 8), byte(top)})
	upper := netip.PrefixFrom(upperAddr, half).Masked()

	return newFrom(lower), newFrom(upper), nil
}

func newFrom(p netip.Prefix) *Allocator {
	return &Allocator{
		prefix: p,
		next:   p.Addr().Next(),
		byKey:  map[string]netip.Addr{},
		used:   map[netip.Addr]bool{},
	}
}

// Prefix is the range this allocator hands out from.
func (a *Allocator) Prefix() netip.Prefix { return a.prefix }

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
