package l4

import (
	"io"
	"net"
	"testing"
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

	p, err := ListenTCP("127.0.0.1:0", backend)
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
