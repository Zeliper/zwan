// Package vip gives each joined network an address space that is unique on this
// device (design doc 38).
//
// Two networks may legitimately use the same overlay range — the default is the
// same for everyone — but the host has one routing table, so the same address
// cannot mean two different peers. Every network therefore gets a local range,
// and addresses are translated at the tunnel boundary: the operating system only
// ever sees local addresses, and the real overlay addresses exist only inside
// the tunnel, where they are unambiguous again.
package vip

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sync"
)

// Table is one network's two-way mapping between real overlay addresses and the
// local addresses this device uses for them.
type Table struct {
	prefix netip.Prefix // local range handed out from
	real   netip.Prefix // the network's own overlay range
	base   uint32       // first address in the local range, as a host-order integer
	size   uint32       // number of addresses in the local range

	mu      sync.RWMutex
	toLocal map[netip.Addr]netip.Addr
	toReal  map[netip.Addr]netip.Addr
	used    map[uint32]bool // host indexes handed out
	next    uint32          // scan cursor for the fallback allocator
}

// NewTable creates a mapping from a network's overlay range to a local range
// that is unique on this device. The local range needs at least four addresses
// so there is something to hand out once the network and broadcast addresses are
// set aside.
//
// Naming the overlay range is what keeps the table honest: only addresses that
// belong to this network can be mapped, so a peer cannot make traffic appear to
// come from somewhere else in the local space.
func NewTable(local, real netip.Prefix) (*Table, error) {
	local, real = local.Masked(), real.Masked()
	if !local.Addr().Is4() {
		return nil, fmt.Errorf("local range %s must be IPv4", local)
	}
	if !real.Addr().Is4() {
		return nil, fmt.Errorf("overlay range %s must be IPv4", real)
	}
	bits := local.Bits()
	if bits > 30 {
		return nil, fmt.Errorf("local range %s is too small", local)
	}
	return &Table{
		prefix:  local,
		real:    real,
		base:    toUint32(local.Addr()),
		size:    uint32(1) << uint(32-bits),
		toLocal: map[netip.Addr]netip.Addr{},
		toReal:  map[netip.Addr]netip.Addr{},
		used:    map[uint32]bool{},
		next:    1,
	}, nil
}

// Prefix is the local range this table hands out from.
func (t *Table) Prefix() netip.Prefix { return t.prefix }

// Overlay is the network's own address range.
func (t *Table) Overlay() netip.Prefix { return t.real }

// Local returns the local address for a real overlay address, allocating one on
// first use.
//
// Where it can, the low bits of the real address are kept, so 100.64.0.7 in a
// network mapped to 100.112.0.0/16 becomes 100.112.0.7. That is only a
// convenience for reading logs — when the bits are already taken, or the real
// range is larger than the local one, any free address will do.
func (t *Table) Local(real netip.Addr) (netip.Addr, bool) {
	real = real.Unmap()
	if !real.Is4() || !t.real.Contains(real) {
		return netip.Addr{}, false
	}
	t.mu.RLock()
	if got, ok := t.toLocal[real]; ok {
		t.mu.RUnlock()
		return got, true
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()
	if got, ok := t.toLocal[real]; ok { // won by another caller
		return got, true
	}
	idx, ok := t.allocLocked(real)
	if !ok {
		return netip.Addr{}, false
	}
	local := fromUint32(t.base + idx)
	t.used[idx] = true
	t.toLocal[real] = local
	t.toReal[local] = real
	return local, true
}

// Real returns the overlay address a local address stands for.
func (t *Table) Real(local netip.Addr) (netip.Addr, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	got, ok := t.toReal[local.Unmap()]
	return got, ok
}

// Pairs returns the mapping as real -> local, for status displays.
func (t *Table) Pairs() map[netip.Addr]netip.Addr {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[netip.Addr]netip.Addr, len(t.toLocal))
	for k, v := range t.toLocal {
		out[k] = v
	}
	return out
}

// allocLocked picks a free host index. Callers hold the write lock.
func (t *Table) allocLocked(real netip.Addr) (uint32, bool) {
	if want := toUint32(real) & (t.size - 1); usable(want, t.size) && !t.used[want] {
		return want, true
	}
	for scanned := uint32(0); scanned < t.size; scanned++ {
		idx := t.next
		t.next++
		if t.next >= t.size {
			t.next = 1
		}
		if usable(idx, t.size) && !t.used[idx] {
			return idx, true
		}
	}
	return 0, false
}

// usable keeps the network and broadcast addresses out of circulation.
func usable(idx, size uint32) bool { return idx != 0 && idx != size-1 }

func toUint32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func fromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}
