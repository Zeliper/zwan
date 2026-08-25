package vip

import (
	"net/netip"
	"os"
	"testing"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// fakeTUN stands in for the adapter: Read hands out queued packets, Write
// records what reached the operating system.
type fakeTUN struct {
	toRead  [][]byte
	written [][]byte
	events  chan wgtun.Event
}

func (f *fakeTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n := 0
	for n < len(bufs) && n < len(f.toRead) {
		copy(bufs[n][offset:], f.toRead[n])
		sizes[n] = len(f.toRead[n])
		n++
	}
	f.toRead = nil
	return n, nil
}

func (f *fakeTUN) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		f.written = append(f.written, append([]byte(nil), b[offset:]...))
	}
	return len(bufs), nil
}

func (f *fakeTUN) File() *os.File             { return nil }
func (f *fakeTUN) MTU() (int, error)          { return 1420, nil }
func (f *fakeTUN) Name() (string, error)      { return "fake", nil }
func (f *fakeTUN) Events() <-chan wgtun.Event { return f.events }
func (f *fakeTUN) Close() error               { return nil }
func (f *fakeTUN) BatchSize() int             { return 4 }

func wrapped(t *testing.T) (*Device, *fakeTUN, *Table) {
	t.Helper()
	tbl := mustTable(t, "100.112.0.0/16")
	inner := &fakeTUN{events: make(chan wgtun.Event)}
	dev, local, err := Wrap(inner, tbl, realA)
	if err != nil {
		t.Fatal(err)
	}
	if local != netip.MustParseAddr("100.112.0.1") {
		t.Fatalf("this node's local address = %v", local)
	}
	return dev, inner, tbl
}

func udpPacket(t *testing.T, src, dst netip.Addr) []byte {
	t.Helper()
	udp := make([]byte, 8)
	udp[5] = 8 // length
	return buildIPv4(t, src, dst, protoUDP, udp)
}

// A packet from the operating system leaves in overlay addresses.
func TestReadTranslatesOutOfTheLocalSpace(t *testing.T) {
	dev, inner, tbl := wrapped(t)
	peerLocal, _ := tbl.Local(realB)

	inner.toRead = [][]byte{udpPacket(t, dev.LocalAddr(), peerLocal)}
	bufs := [][]byte{make([]byte, 200)}
	sizes := make([]int, 1)

	n, err := dev.Read(bufs, sizes, 0)
	if err != nil || n != 1 || sizes[0] == 0 {
		t.Fatalf("read = (%d, %v), sizes %v", n, err, sizes)
	}
	pkt := bufs[0][:sizes[0]]
	if addrAt(pkt[12:16]) != realA || addrAt(pkt[16:20]) != realB {
		t.Fatalf("addresses = %v -> %v, want the overlay pair", addrAt(pkt[12:16]), addrAt(pkt[16:20]))
	}
	assertValid(t, pkt, "read")
}

// A packet off the tunnel arrives in local addresses.
func TestWriteTranslatesIntoTheLocalSpace(t *testing.T) {
	dev, inner, _ := wrapped(t)

	pkt := udpPacket(t, realB, realA)
	if _, err := dev.Write([][]byte{pkt}, 0); err != nil {
		t.Fatal(err)
	}
	if len(inner.written) != 1 {
		t.Fatalf("packets reaching the adapter = %d, want 1", len(inner.written))
	}
	got := inner.written[0]
	if addrAt(got[16:20]) != dev.LocalAddr() {
		t.Fatalf("destination = %v, want this node's local address %v", addrAt(got[16:20]), dev.LocalAddr())
	}
	if !addrAt(got[12:16]).IsValid() || addrAt(got[12:16]) == realB {
		t.Fatalf("source = %v, want a translated local address", addrAt(got[12:16]))
	}
	assertValid(t, got, "write")
}

// wireguard-go pairs each buffer with its own state by index, so an untranslatable
// packet is zero-length rather than removed from the batch.
func TestReadZeroesPacketsItCannotTranslate(t *testing.T) {
	dev, inner, tbl := wrapped(t)
	peerLocal, _ := tbl.Local(realB)

	inner.toRead = [][]byte{
		udpPacket(t, dev.LocalAddr(), netip.MustParseAddr("100.112.9.9")), // unmapped
		udpPacket(t, dev.LocalAddr(), peerLocal),
	}
	bufs := [][]byte{make([]byte, 200), make([]byte, 200)}
	sizes := make([]int, 2)

	n, err := dev.Read(bufs, sizes, 0)
	if err != nil || n != 2 {
		t.Fatalf("read = (%d, %v)", n, err)
	}
	if sizes[0] != 0 {
		t.Fatal("an untranslatable packet should be zero-length")
	}
	if sizes[1] == 0 {
		t.Fatal("the translatable packet should have survived")
	}
}

// Writing has no such constraint, so unmappable packets are simply not delivered.
func TestWriteDropsPacketsItCannotTranslate(t *testing.T) {
	dev, inner, _ := wrapped(t)

	stray := udpPacket(t, netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"))
	if _, err := dev.Write([][]byte{stray}, 0); err != nil {
		t.Fatal(err)
	}
	if len(inner.written) != 0 {
		t.Fatalf("an untranslatable packet reached the adapter: %v", inner.written)
	}
}

// A packet through both directions comes back byte for byte.
func TestRoundTripThroughTheDevice(t *testing.T) {
	dev, inner, tbl := wrapped(t)
	peerLocal, _ := tbl.Local(realB)

	original := udpPacket(t, dev.LocalAddr(), peerLocal)
	inner.toRead = [][]byte{append([]byte(nil), original...)}
	bufs := [][]byte{make([]byte, 200)}
	sizes := make([]int, 1)
	if _, err := dev.Read(bufs, sizes, 0); err != nil {
		t.Fatal(err)
	}
	onTheWire := append([]byte(nil), bufs[0][:sizes[0]]...)

	// Send the same packet back the other way.
	if _, err := dev.Write([][]byte{onTheWire}, 0); err != nil {
		t.Fatal(err)
	}
	if got := inner.written[0]; string(got) != string(original) {
		t.Fatal("a round trip did not reproduce the original packet")
	}
}

func TestWrapFailsWhenTheRangeIsFull(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/30")
	_, _ = tbl.Local(netip.MustParseAddr("100.64.0.8"))
	_, _ = tbl.Local(netip.MustParseAddr("100.64.0.9"))

	if _, _, err := Wrap(&fakeTUN{}, tbl, realA); err == nil {
		t.Fatal("wrapping should fail when no local address is left")
	}
}
