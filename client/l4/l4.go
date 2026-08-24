// Package l4 is the L4 service router. On the hosting node it accepts overlay
// connections to a service and forwards them to the real backend, which can stay
// bound to localhost only (never exposed on the physical LAN or the internet).
//
// The hosting node is also where a service's access list is enforced. The
// control server decides who is told a service exists, but a member that already
// knows the address and port would otherwise still get through; checking the
// source here is what makes an access list a boundary rather than a hint.
package l4

import (
	"io"
	"net"
	"net/netip"
	"sync"
)

// TCPProxy forwards accepted TCP connections to a fixed backend address.
type TCPProxy struct {
	ln      net.Listener
	backend string
	allow   func(netip.Addr) bool
}

// ListenTCP starts a proxy on listenAddr that forwards to backend (host:port).
//
// allow, when non-nil, is asked about the source address of every accepted
// connection; a refused source is closed before a single byte reaches the
// backend. Pass nil to accept every source.
func ListenTCP(listenAddr, backend string, allow func(netip.Addr) bool) (*TCPProxy, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	p := &TCPProxy{ln: ln, backend: backend, allow: allow}
	go p.serve()
	return p, nil
}

// Addr is the address the proxy listens on.
func (p *TCPProxy) Addr() net.Addr { return p.ln.Addr() }

// Close stops accepting new connections.
func (p *TCPProxy) Close() error { return p.ln.Close() }

func (p *TCPProxy) serve() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		if !p.permit(c.RemoteAddr()) {
			_ = c.Close()
			continue
		}
		go p.handle(c)
	}
}

// permit applies the access check. An address that cannot be parsed is refused
// rather than allowed: the check exists to keep people out.
func (p *TCPProxy) permit(addr net.Addr) bool {
	if p.allow == nil {
		return true
	}
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return false
	}
	return p.allow(ap.Addr().Unmap())
}

func (p *TCPProxy) handle(client net.Conn) {
	defer client.Close()
	backend, err := net.Dial("tcp", p.backend)
	if err != nil {
		return
	}
	defer backend.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(backend, client)
		if b, ok := backend.(*net.TCPConn); ok {
			_ = b.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, backend)
		if c, ok := client.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
	}()
	wg.Wait()
}
