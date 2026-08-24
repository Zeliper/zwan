// Package tun creates and configures the OS virtual network adapter (Wintun on
// Windows) that carries a zwan overlay network's traffic.
//
// M1b-1: create the adapter and assign the overlay IP. The wireguard-go device
// that reads/writes packets on this adapter is wired up in M1b-2.
package tun

import (
	"fmt"

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

// SetNodeIP assigns the node's overlay address to the adapter as a /32.
//
// A /32 (host route) is used deliberately: the overlay CIDR is not put on-link,
// so per-peer /32 routes (AddPeerRoute) drive which destinations enter the
// tunnel. This also lets multiple adapters in the same overlay CIDR coexist on
// one host (needed for local two-client tests and multi-network clients).
func (a *Adapter) SetNodeIP(ip string) error {
	return setIPv4(a.Name, ip, 32)
}

// AddPeerRoute adds a /32 route for a peer's overlay address via this adapter.
func (a *Adapter) AddPeerRoute(peerIP string) error {
	return addRoute(a.Name, peerIP)
}

// Close removes the adapter.
func (a *Adapter) Close() error {
	if a.Dev == nil {
		return nil
	}
	return a.Dev.Close()
}
