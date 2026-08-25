package l4

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// udpIdle is how long a flow is kept after the backend stops answering.
//
// UDP has no close, so a flow only ends by going quiet. The timeout is long
// enough that a game's lull between packets does not lose the session, and short
// enough that a machine does not accumulate sockets for clients that left.
const udpIdle = 90 * time.Second

// UDPProxy forwards datagrams arriving on an overlay address to a backend.
//
// Each client gets its own socket to the backend, which is what makes replies
// findable: the backend answers on that socket, and the proxy knows which client
// it belongs to. That mapping is the whole of the state, and it expires on its
// own.
type UDPProxy struct {
	pc      *net.UDPConn
	backend *net.UDPAddr
	allow   func(netip.Addr) bool

	mu     sync.Mutex
	flows  map[netip.AddrPort]*net.UDPConn
	closed bool
}

// ListenUDP starts a proxy on listenAddr that forwards to backend (host:port).
// allow behaves as it does for ListenTCP: nil accepts every source.
func ListenUDP(listenAddr, backend string, allow func(netip.Addr) bool) (*UDPProxy, error) {
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	baddr, err := net.ResolveUDPAddr("udp", backend)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}
	p := &UDPProxy{pc: pc, backend: baddr, allow: allow, flows: map[netip.AddrPort]*net.UDPConn{}}
	go p.serve()
	return p, nil
}

// Addr is the address the proxy listens on.
func (p *UDPProxy) Addr() net.Addr { return p.pc.LocalAddr() }

// Close stops the proxy and drops every flow.
func (p *UDPProxy) Close() error {
	p.mu.Lock()
	p.closed = true
	flows := p.flows
	p.flows = map[netip.AddrPort]*net.UDPConn{}
	p.mu.Unlock()

	for _, c := range flows {
		_ = c.Close()
	}
	return p.pc.Close()
}

func (p *UDPProxy) serve() {
	buf := make([]byte, 64*1024)
	for {
		n, src, err := p.pc.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		if p.allow != nil && !p.allow(src.Addr().Unmap()) {
			continue
		}
		conn, err := p.flowFor(src)
		if err != nil {
			continue
		}
		_, _ = conn.Write(buf[:n])
	}
}

// flowFor returns this client's socket to the backend, opening one on first use
// and pushing back its expiry on every packet.
func (p *UDPProxy) flowFor(src netip.AddrPort) (*net.UDPConn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, net.ErrClosed
	}
	if c, ok := p.flows[src]; ok {
		p.mu.Unlock()
		_ = c.SetReadDeadline(time.Now().Add(udpIdle))
		return c, nil
	}
	p.mu.Unlock()

	conn, err := net.DialUDP("udp", nil, p.backend)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	if existing, ok := p.flows[src]; ok { // won by another datagram
		p.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	p.flows[src] = conn
	p.mu.Unlock()

	go p.pump(src, conn)
	return conn, nil
}

// pump carries the backend's replies back to one client until the flow goes
// quiet for udpIdle.
func (p *UDPProxy) pump(src netip.AddrPort, conn *net.UDPConn) {
	defer func() {
		p.mu.Lock()
		if p.flows[src] == conn {
			delete(p.flows, src)
		}
		p.mu.Unlock()
		_ = conn.Close()
	}()

	buf := make([]byte, 64*1024)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(udpIdle))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		if _, err := p.pc.WriteToUDPAddrPort(buf[:n], src); err != nil {
			return
		}
	}
}
