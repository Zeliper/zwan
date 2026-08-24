// Package l4 is the L4 service router. On the hosting node it accepts overlay
// connections to a service and forwards them to the real backend, which can stay
// bound to localhost only (never exposed on the physical LAN or the internet).
package l4

import (
	"io"
	"net"
	"sync"
)

// TCPProxy forwards accepted TCP connections to a fixed backend address.
type TCPProxy struct {
	ln      net.Listener
	backend string
}

// ListenTCP starts a proxy on listenAddr that forwards to backend (host:port).
func ListenTCP(listenAddr, backend string) (*TCPProxy, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	p := &TCPProxy{ln: ln, backend: backend}
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
		go p.handle(c)
	}
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
