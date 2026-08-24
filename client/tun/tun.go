// Package tun creates and configures the OS virtual network adapter (Wintun on
// Windows) that carries a zwan overlay network's traffic.
//
// M1b-1: create the adapter and assign the overlay IP. The wireguard-go device
// that reads/writes packets on this adapter is wired up in M1b-2.
package tun

import (
	"fmt"
	"net/netip"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// DefaultMTU is the standard WireGuard tunnel MTU.
const DefaultMTU = 1420

// Adapter is a created virtual NIC.
type Adapter struct {
	Name string
	Dev  wgtun.Device
}

// Create makes a new virtual adapter named name (e.g. "MyWAN-demo").
//
// On Windows this creates a Wintun adapter and requires Administrator rights and
// wintun.dll to be resolvable (next to the binary, in System32, or on PATH).
func Create(name string, mtu int) (*Adapter, error) {
	if mtu <= 0 {
		mtu = DefaultMTU
	}
	dev, err := wgtun.CreateTUN(name, mtu)
	if err != nil {
		return nil, fmt.Errorf("create adapter %q: %w", name, err)
	}
	return &Adapter{Name: name, Dev: dev}, nil
}

// SetOverlayIP assigns a single overlay address to the adapter, taking the
// prefix length from the network's CIDR (e.g. ip=100.64.0.1, cidr=100.64.0.0/16).
func (a *Adapter) SetOverlayIP(ip, cidr string) error {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %q: %w", cidr, err)
	}
	return setIPv4(a.Name, ip, p.Bits())
}

// Close removes the adapter.
func (a *Adapter) Close() error {
	if a.Dev == nil {
		return nil
	}
	return a.Dev.Close()
}
