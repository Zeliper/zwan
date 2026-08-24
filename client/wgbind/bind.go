// Package wgbind is a minimal wireguard-go conn.Bind over IPv4/UDP.
//
// It exists to fix a Windows-specific failure: the stock binds do not disable
// SIO_UDP_CONNRESET, so when a WireGuard handshake packet is sent to a peer that
// is not listening yet, Windows raises WSAECONNRESET on the next receive and the
// wireguard-go receive routine dies — the tunnel then never establishes. This
// bind disables that behaviour (and also swallows the reset defensively).
//
// It is intentionally simple (batch size 1, IPv4 only). It is also the seam
// where relay transport will hook in (M1b-3): today it sends straight to the
// peer endpoint; later it can tunnel through the server relay.
package wgbind

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
)

// Bind implements conn.Bind.
type Bind struct {
	mu sync.Mutex
	uc *net.UDPConn
}

// New returns a fresh bind.
func New() *Bind { return &Bind{} }

// Open binds the UDP socket (with SIO_UDP_CONNRESET disabled on Windows).
func (b *Bind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.uc != nil {
		return nil, 0, errors.New("wgbind: already open")
	}
	lc := net.ListenConfig{Control: controlDisableConnReset}
	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, 0, err
	}
	uc := pc.(*net.UDPConn)
	b.uc = uc
	actual := uint16(uc.LocalAddr().(*net.UDPAddr).Port)
	return []conn.ReceiveFunc{b.makeReceive(uc)}, actual, nil
}

func (b *Bind) makeReceive(uc *net.UDPConn) conn.ReceiveFunc {
	return func(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		for {
			n, ap, err := uc.ReadFromUDPAddrPort(bufs[0])
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return 0, net.ErrClosed
				}
				if isConnReset(err) {
					continue // ignore ICMP-port-unreachable induced resets
				}
				return 0, err
			}
			sizes[0] = n
			eps[0] = &conn.StdNetEndpoint{AddrPort: ap}
			return 1, nil
		}
	}
}

// Send transmits each buffer to the endpoint.
func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	se, ok := ep.(*conn.StdNetEndpoint)
	if !ok {
		return fmt.Errorf("wgbind: unexpected endpoint type %T", ep)
	}
	b.mu.Lock()
	uc := b.uc
	b.mu.Unlock()
	if uc == nil {
		return net.ErrClosed
	}
	for _, buf := range bufs {
		if _, err := uc.WriteToUDPAddrPort(buf, se.AddrPort); err != nil {
			return err
		}
	}
	return nil
}

// ParseEndpoint parses an "ip:port" endpoint.
func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return &conn.StdNetEndpoint{AddrPort: ap}, nil
}

// Close closes the socket.
func (b *Bind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.uc == nil {
		return nil
	}
	err := b.uc.Close()
	b.uc = nil
	return err
}

// SetMark is a no-op (fwmark is a Linux concept).
func (b *Bind) SetMark(uint32) error { return nil }

// BatchSize returns 1 (no GSO batching).
func (b *Bind) BatchSize() int { return 1 }
