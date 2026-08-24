// Package relay is the server-side packet relay: it forwards opaque (encrypted)
// WireGuard packets between clients by overlay IP, so clients behind NAT that
// cannot reach each other directly can still communicate via the always-on,
// public-IP server.
//
// Wire format (client <-> relay), IPv4 overlay addresses:
//
//	register/keepalive : [0x01][selfIP:4]
//	data (to relay)    : [0x02][srcIP:4][dstIP:4][ WireGuard packet ]
//	data (from relay)  : [0x02][srcIP:4][ WireGuard packet ]
//
// The relay learns each client's current UDP address from the packets it sends
// and forwards by destination overlay IP. It never sees plaintext: the payload
// is an end-to-end encrypted WireGuard packet.
package relay

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	typeRegister = 0x01
	typeData     = 0x02
)

type client struct {
	addr     netip.AddrPort
	lastSeen time.Time
}

// Relay forwards packets between clients by overlay IP.
type Relay struct {
	mu      sync.Mutex
	clients map[netip.Addr]client
	conn    *net.UDPConn
}

// New creates an empty relay.
func New() *Relay { return &Relay{clients: map[netip.Addr]client{}} }

// Listen binds addr (host:port, UDP) and returns the actual bound address.
func (r *Relay) Listen(addr string) (netip.AddrPort, error) {
	uaddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return netip.AddrPort{}, err
	}
	conn, err := net.ListenUDP("udp4", uaddr)
	if err != nil {
		return netip.AddrPort{}, err
	}
	r.conn = conn
	return conn.LocalAddr().(*net.UDPAddr).AddrPort(), nil
}

// Serve reads and forwards packets until the socket is closed.
func (r *Relay) Serve() error {
	buf := make([]byte, 1500)
	for {
		n, src, err := r.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return err
		}
		r.handle(buf[:n], src)
	}
}

// ListenAndServe binds addr and serves until the socket closes.
func (r *Relay) ListenAndServe(addr string) error {
	if _, err := r.Listen(addr); err != nil {
		return err
	}
	return r.Serve()
}

// Close closes the relay socket.
func (r *Relay) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

func (r *Relay) handle(pkt []byte, src netip.AddrPort) {
	if len(pkt) < 1 {
		return
	}
	switch pkt[0] {
	case typeRegister:
		if len(pkt) < 5 {
			return
		}
		ip := netip.AddrFrom4([4]byte(pkt[1:5]))
		r.mu.Lock()
		r.clients[ip] = client{addr: src, lastSeen: time.Now()}
		r.mu.Unlock()

	case typeData:
		if len(pkt) < 9 {
			return
		}
		srcIP := netip.AddrFrom4([4]byte(pkt[1:5]))
		dstIP := netip.AddrFrom4([4]byte(pkt[5:9]))
		payload := pkt[9:]

		r.mu.Lock()
		r.clients[srcIP] = client{addr: src, lastSeen: time.Now()} // learn/refresh sender
		dst, ok := r.clients[dstIP]
		r.mu.Unlock()
		if !ok {
			return // destination not connected; drop
		}

		out := make([]byte, 0, 5+len(payload))
		out = append(out, typeData)
		s4 := srcIP.As4()
		out = append(out, s4[:]...)
		out = append(out, payload...)
		_, _ = r.conn.WriteToUDPAddrPort(out, dst.addr)
	}
}
