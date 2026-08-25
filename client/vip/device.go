package vip

import (
	"fmt"
	"net/netip"
	"os"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// Device wraps a TUN device so the operating system only ever sees this
// network's local addresses, while the tunnel only ever carries the real ones.
//
// wireguard-go accepts any tun.Device, so the translation sits exactly at the
// boundary between the two address spaces and nothing above or below it needs to
// know that two networks share a range.
type Device struct {
	inner     wgtun.Device
	table     *Table
	selfReal  netip.Addr
	selfLocal netip.Addr
}

// Wrap puts a translating layer in front of a TUN device. selfReal is this
// node's overlay address; its local counterpart is what the adapter is
// configured with and is returned for the caller to assign.
func Wrap(inner wgtun.Device, table *Table, selfReal netip.Addr) (*Device, netip.Addr, error) {
	selfReal = selfReal.Unmap()
	local, ok := table.Local(selfReal)
	if !ok {
		return nil, netip.Addr{}, fmt.Errorf("no local address available in %s", table.Prefix())
	}
	return &Device{inner: inner, table: table, selfReal: selfReal, selfLocal: local}, local, nil
}

// LocalAddr is this node's address in the local space.
func (d *Device) LocalAddr() netip.Addr { return d.selfLocal }

// Read takes packets from the operating system and turns their local addresses
// into overlay addresses, so WireGuard sees the addresses its peers expect.
//
// A packet nothing maps to gets a zero length rather than being removed:
// wireguard-go pairs each buffer with its own state by index, and skips any
// entry whose size is zero.
func (d *Device) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n, err := d.inner.Read(bufs, sizes, offset)
	for i := 0; i < n; i++ {
		if sizes[i] <= 0 {
			continue
		}
		if !rewrite(bufs[i][offset:offset+sizes[i]], d.toReal) {
			sizes[i] = 0
		}
	}
	return n, err
}

// Write takes packets off the tunnel and turns their overlay addresses into
// local ones before the operating system sees them. Packets with no mapping are
// dropped, which is why the count written can be lower than the count offered.
func (d *Device) Write(bufs [][]byte, offset int) (int, error) {
	keep := bufs[:0:0] // a fresh slice: bufs belongs to the caller
	for _, b := range bufs {
		if len(b) <= offset {
			continue
		}
		if rewrite(b[offset:], d.toLocal) {
			keep = append(keep, b)
		}
	}
	if len(keep) == 0 {
		return 0, nil
	}
	return d.inner.Write(keep, offset)
}

// toReal translates an address coming from the operating system.
func (d *Device) toReal(a netip.Addr) (netip.Addr, bool) {
	if a == d.selfLocal {
		return d.selfReal, true
	}
	return d.table.Real(a)
}

// toLocal translates an address arriving from a peer. Unlike toReal it may
// allocate: a peer we have a route for can still be sending us its first packet.
func (d *Device) toLocal(a netip.Addr) (netip.Addr, bool) {
	if a == d.selfReal {
		return d.selfLocal, true
	}
	return d.table.Local(a)
}

func (d *Device) File() *os.File             { return d.inner.File() }
func (d *Device) MTU() (int, error)          { return d.inner.MTU() }
func (d *Device) Name() (string, error)      { return d.inner.Name() }
func (d *Device) Events() <-chan wgtun.Event { return d.inner.Events() }
func (d *Device) Close() error               { return d.inner.Close() }
func (d *Device) BatchSize() int             { return d.inner.BatchSize() }
