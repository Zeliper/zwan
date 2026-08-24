// Package wgbind is a minimal wireguard-go conn.Bind over IPv4/UDP with two modes.
//
// Direct mode (New): sends WireGuard packets straight to the peer's UDP endpoint.
//
// Relay mode (NewRelay): wraps each WireGuard packet with overlay-IP framing and
// sends it to the server relay, which forwards it to the destination client. This
// lets peers behind NAT communicate via the always-on public-IP server. See
// server/relay for the wire format.
//
// Either way it disables Windows' SIO_UDP_CONNRESET so an ICMP "port unreachable"
// from a not-yet-listening peer does not kill the receive routine.
package wgbind

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

const (
	typeRegister = 0x01
	typeData     = 0x02
)

// Bind implements conn.Bind.
type Bind struct {
	mu sync.Mutex
	uc *net.UDPConn

	relay   bool
	relayAP netip.AddrPort
	selfIP  netip.Addr
	stop    chan struct{}
}

// New returns a direct-mode bind.
func New() *Bind { return &Bind{} }

// NewRelay returns a relay-mode bind that tunnels through the server relay at
// relayAddr, identifying this node by its IPv4 overlay address selfIP.
func NewRelay(relayAddr string, selfIP netip.Addr) (*Bind, error) {
	ap, err := netip.ParseAddrPort(relayAddr)
	if err != nil {
		return nil, fmt.Errorf("relay addr %q: %w", relayAddr, err)
	}
	if !selfIP.Is4() {
		return nil, fmt.Errorf("relay: self overlay IP must be IPv4, got %s", selfIP)
	}
	return &Bind{relay: true, relayAP: ap, selfIP: selfIP}, nil
}

// Open binds the UDP socket (SIO_UDP_CONNRESET disabled on Windows).
func (b *Bind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.uc != nil {
		return nil, 0, errors.New("wgbind: already open")
	}
	bindPort := port
	if b.relay {
		bindPort = 0 // relay client uses an ephemeral local port
	}
	lc := net.ListenConfig{Control: controlDisableConnReset}
	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", bindPort))
	if err != nil {
		return nil, 0, err
	}
	uc := pc.(*net.UDPConn)
	b.uc = uc
	actual := uint16(uc.LocalAddr().(*net.UDPAddr).Port)

	if b.relay {
		b.stop = make(chan struct{})
		b.sendRegister(uc)
		go b.keepalive(uc)
		return []conn.ReceiveFunc{b.makeRelayReceive(uc)}, actual, nil
	}
	return []conn.ReceiveFunc{b.makeDirectReceive(uc)}, actual, nil
}

func (b *Bind) makeDirectReceive(uc *net.UDPConn) conn.ReceiveFunc {
	return func(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		for {
			n, ap, err := uc.ReadFromUDPAddrPort(bufs[0])
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return 0, net.ErrClosed
				}
				if isConnReset(err) {
					continue
				}
				return 0, err
			}
			sizes[0] = n
			eps[0] = &conn.StdNetEndpoint{AddrPort: ap}
			return 1, nil
		}
	}
}

func (b *Bind) makeRelayReceive(uc *net.UDPConn) conn.ReceiveFunc {
	return func(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		for {
			n, _, err := uc.ReadFromUDPAddrPort(bufs[0])
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return 0, net.ErrClosed
				}
				if isConnReset(err) {
					continue
				}
				return 0, err
			}
			if n < 5 || bufs[0][0] != typeData {
				continue // register ack / malformed
			}
			srcIP := netip.AddrFrom4([4]byte(bufs[0][1:5]))
			copy(bufs[0], bufs[0][5:n]) // shift WireGuard payload to the front
			sizes[0] = n - 5
			eps[0] = &relayEndpoint{ip: srcIP}
			return 1, nil
		}
	}
}

// Send transmits each buffer to the endpoint (directly or via the relay).
func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	b.mu.Lock()
	uc := b.uc
	b.mu.Unlock()
	if uc == nil {
		return net.ErrClosed
	}

	if b.relay {
		re, ok := ep.(*relayEndpoint)
		if !ok {
			return fmt.Errorf("wgbind: expected relayEndpoint, got %T", ep)
		}
		s4, d4 := b.selfIP.As4(), re.ip.As4()
		for _, buf := range bufs {
			frame := make([]byte, 0, 9+len(buf))
			frame = append(frame, typeData)
			frame = append(frame, s4[:]...)
			frame = append(frame, d4[:]...)
			frame = append(frame, buf...)
			if _, err := uc.WriteToUDPAddrPort(frame, b.relayAP); err != nil {
				return err
			}
		}
		return nil
	}

	se, ok := ep.(*conn.StdNetEndpoint)
	if !ok {
		return fmt.Errorf("wgbind: expected StdNetEndpoint, got %T", ep)
	}
	for _, buf := range bufs {
		if _, err := uc.WriteToUDPAddrPort(buf, se.AddrPort); err != nil {
			return err
		}
	}
	return nil
}

// ParseEndpoint parses an endpoint. In relay mode the string is the peer's
// overlay IP (optionally "ip:port"); in direct mode it is "ip:port".
func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	if b.relay {
		if ap, err := netip.ParseAddrPort(s); err == nil {
			return &relayEndpoint{ip: ap.Addr()}, nil
		}
		ip, err := netip.ParseAddr(s)
		if err != nil {
			return nil, err
		}
		return &relayEndpoint{ip: ip}, nil
	}
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return &conn.StdNetEndpoint{AddrPort: ap}, nil
}

func (b *Bind) sendRegister(uc *net.UDPConn) {
	s4 := b.selfIP.As4()
	frame := append([]byte{typeRegister}, s4[:]...)
	_, _ = uc.WriteToUDPAddrPort(frame, b.relayAP)
}

func (b *Bind) keepalive(uc *net.UDPConn) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.sendRegister(uc)
		}
	}
}

// Close closes the socket.
func (b *Bind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stop != nil {
		close(b.stop)
		b.stop = nil
	}
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

// relayEndpoint identifies a peer by its overlay IP for relay routing.
type relayEndpoint struct{ ip netip.Addr }

func (e *relayEndpoint) ClearSrc()             {}
func (e *relayEndpoint) SrcToString() string   { return "" }
func (e *relayEndpoint) DstToString() string   { return e.ip.String() }
func (e *relayEndpoint) DstToBytes() []byte    { b := e.ip.As4(); return b[:] }
func (e *relayEndpoint) DstIP() netip.Addr     { return e.ip }
func (e *relayEndpoint) SrcIP() netip.Addr     { return netip.Addr{} }
