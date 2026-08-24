package l4

import (
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// echoServer accepts TCP and echoes bytes back; returns its address.
func echoServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestTCPProxyForwards(t *testing.T) {
	backend, stop := echoServer(t)
	defer stop()

	p, err := ListenTCP("127.0.0.1:0", backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	conn, err := net.Dial("tcp", p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := []byte("hello over l4")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("got %q, want %q", buf, msg)
	}
}

// dialAndRead reports whether the proxy carried a request through to the backend.
func dialAndRead(t *testing.T, addr string) bool {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello")); err != nil {
		return false
	}
	buf := make([]byte, 5)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := io.ReadFull(conn, buf)
	return err == nil && n == 5
}

// A refused source is dropped at accept, so the backend never sees it.
func TestTCPProxyRefusesDisallowedSources(t *testing.T) {
	backend, stop := echoServer(t)
	defer stop()

	var asked []netip.Addr
	var mu sync.Mutex
	allowed := true
	p, err := ListenTCP("127.0.0.1:0", backend, func(src netip.Addr) bool {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, src)
		return allowed
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if !dialAndRead(t, p.Addr().String()) {
		t.Fatal("an allowed source should be forwarded")
	}

	mu.Lock()
	allowed = false
	mu.Unlock()
	if dialAndRead(t, p.Addr().String()) {
		t.Fatal("a refused source must not reach the backend")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 2 {
		t.Fatalf("the filter should be consulted once per connection, got %d", len(asked))
	}
	if !asked[0].IsLoopback() {
		t.Fatalf("the filter saw %v, want the loopback source address", asked[0])
	}
}

// A nil filter keeps the previous behaviour: every source is accepted.
func TestTCPProxyWithoutFilterAcceptsEveryone(t *testing.T) {
	backend, stop := echoServer(t)
	defer stop()

	p, err := ListenTCP("127.0.0.1:0", backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if !dialAndRead(t, p.Addr().String()) {
		t.Fatal("an unfiltered proxy should forward")
	}
}
