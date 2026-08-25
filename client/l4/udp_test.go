package l4

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

// udpEcho answers every datagram with the same bytes, prefixed so a test can
// tell the backend answered rather than the proxy looping.
func udpEcho(t *testing.T) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, src, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(append([]byte("re:"), buf[:n]...), src)
		}
	}()
	return pc.LocalAddr().String(), func() { pc.Close() }
}

// exchange sends one datagram through the proxy and waits for the reply.
func exchange(t *testing.T, proxyAddr, msg string) (string, bool) {
	t.Helper()
	c, err := net.Dial("udp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.Read(buf)
	if err != nil {
		return "", false
	}
	return string(buf[:n]), true
}

func TestUDPProxyForwardsBothWays(t *testing.T) {
	backend, stop := udpEcho(t)
	defer stop()

	p, err := ListenUDP("127.0.0.1:0", backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	got, ok := exchange(t, p.Addr().String(), "hello")
	if !ok || got != "re:hello" {
		t.Fatalf("reply = %q (ok=%v)", got, ok)
	}
}

// Two clients must not see each other's replies, which is the whole reason each
// gets its own socket to the backend.
func TestUDPProxyKeepsClientsApart(t *testing.T) {
	backend, stop := udpEcho(t)
	defer stop()

	p, err := ListenUDP("127.0.0.1:0", backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	first, ok1 := exchange(t, p.Addr().String(), "one")
	second, ok2 := exchange(t, p.Addr().String(), "two")
	if !ok1 || !ok2 {
		t.Fatalf("both clients should get a reply: %v %v", ok1, ok2)
	}
	if first != "re:one" || second != "re:two" {
		t.Fatalf("replies crossed: %q %q", first, second)
	}
}

// The access check applies to datagrams the same way it applies to connections.
func TestUDPProxyRefusesDisallowedSources(t *testing.T) {
	backend, stop := udpEcho(t)
	defer stop()

	p, err := ListenUDP("127.0.0.1:0", backend, func(netip.Addr) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if got, ok := exchange(t, p.Addr().String(), "hello"); ok {
		t.Fatalf("a refused source got a reply: %q", got)
	}
}

func TestUDPProxyStopsAfterClose(t *testing.T) {
	backend, stop := udpEcho(t)
	defer stop()

	p, err := ListenUDP("127.0.0.1:0", backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	addr := p.Addr().String()
	if _, ok := exchange(t, addr, "hello"); !ok {
		t.Fatal("the proxy should work before it is closed")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if got, ok := exchange(t, addr, "hello"); ok {
		t.Fatalf("a closed proxy still answered: %q", got)
	}
}
