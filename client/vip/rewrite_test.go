package vip

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

// ones is the ones-complement sum used by every checksum in IPv4. A block that
// contains its own correct checksum sums to 0xffff, which is how the tests below
// check a rewritten packet without trusting the code that rewrote it.
func ones(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(sum)
}

func checksum(b []byte) uint16 { return ^ones(b) }

// buildIPv4 assembles a packet with correct IP and transport checksums.
func buildIPv4(t *testing.T, src, dst netip.Addr, proto byte, l4 []byte) []byte {
	t.Helper()
	pkt := make([]byte, 20+len(l4))
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	pkt[8] = 64
	pkt[9] = proto
	s, d := src.As4(), dst.As4()
	copy(pkt[12:16], s[:])
	copy(pkt[16:20], d[:])
	put16(pkt[10:12], checksum(pkt[:20]))
	copy(pkt[20:], l4)

	switch proto {
	case protoTCP:
		put16(pkt[20+16:20+18], 0)
		put16(pkt[20+16:20+18], transportChecksum(pkt))
	case protoUDP:
		put16(pkt[20+6:20+8], 0)
		put16(pkt[20+6:20+8], transportChecksum(pkt))
	case protoICMP:
		put16(pkt[20+2:20+4], 0)
		put16(pkt[20+2:20+4], checksum(pkt[20:]))
	}
	return pkt
}

// transportChecksum computes a TCP/UDP checksum over the pseudo-header.
func transportChecksum(pkt []byte) uint16 {
	l4 := pkt[20:]
	pseudo := make([]byte, 12+len(l4))
	copy(pseudo[0:4], pkt[12:16])
	copy(pseudo[4:8], pkt[16:20])
	pseudo[9] = pkt[9]
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(l4)))
	copy(pseudo[12:], l4)
	return checksum(pseudo)
}

// assertValid recomputes every checksum in a packet from scratch.
func assertValid(t *testing.T, pkt []byte, what string) {
	t.Helper()
	if got := ones(pkt[:20]); got != 0xffff {
		t.Fatalf("%s: IPv4 header checksum is wrong (sum %#04x)", what, got)
	}
	l4 := pkt[20:]
	switch pkt[9] {
	case protoTCP, protoUDP:
		if pkt[9] == protoUDP && get16(l4[6:8]) == 0 {
			return // no checksum in use
		}
		pseudo := make([]byte, 12+len(l4))
		copy(pseudo[0:4], pkt[12:16])
		copy(pseudo[4:8], pkt[16:20])
		pseudo[9] = pkt[9]
		binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(l4)))
		copy(pseudo[12:], l4)
		if got := ones(pseudo); got != 0xffff {
			t.Fatalf("%s: transport checksum is wrong (sum %#04x)", what, got)
		}
	case protoICMP:
		if got := ones(l4); got != 0xffff {
			t.Fatalf("%s: ICMP checksum is wrong (sum %#04x)", what, got)
		}
	}
}

// swap is the translation used by the tests: a fixed two-way substitution.
func swap(pairs map[netip.Addr]netip.Addr) mapFunc {
	return func(a netip.Addr) (netip.Addr, bool) {
		got, ok := pairs[a]
		return got, ok
	}
}

var (
	realA  = netip.MustParseAddr("100.64.0.1")
	realB  = netip.MustParseAddr("100.64.0.2")
	localA = netip.MustParseAddr("100.112.0.1")
	localB = netip.MustParseAddr("100.112.0.2")
)

func toReal() mapFunc {
	return swap(map[netip.Addr]netip.Addr{localA: realA, localB: realB})
}

func toLocal() mapFunc {
	return swap(map[netip.Addr]netip.Addr{realA: localA, realB: localB})
}

func TestRewriteTCPKeepsChecksumsValid(t *testing.T) {
	tcp := make([]byte, 20)
	binary.BigEndian.PutUint16(tcp[0:2], 44321) // source port
	binary.BigEndian.PutUint16(tcp[2:4], 25565) // destination port
	tcp[12] = 0x50
	pkt := buildIPv4(t, localA, localB, protoTCP, tcp)
	assertValid(t, pkt, "before")

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused a packet it has mappings for")
	}
	if addrAt(pkt[12:16]) != realA || addrAt(pkt[16:20]) != realB {
		t.Fatalf("addresses = %v -> %v", addrAt(pkt[12:16]), addrAt(pkt[16:20]))
	}
	assertValid(t, pkt, "after")

	// And back again, which must reproduce the original packet exactly.
	if !rewrite(pkt, toLocal()) {
		t.Fatal("reverse rewrite refused")
	}
	assertValid(t, pkt, "round trip")
	original := buildIPv4(t, localA, localB, protoTCP, tcp)
	if string(pkt) != string(original) {
		t.Fatal("a round trip did not reproduce the original packet")
	}
}

func TestRewriteUDPKeepsChecksumValid(t *testing.T) {
	udp := make([]byte, 8+4)
	binary.BigEndian.PutUint16(udp[0:2], 5353)
	binary.BigEndian.PutUint16(udp[2:4], 53)
	binary.BigEndian.PutUint16(udp[4:6], uint16(len(udp)))
	copy(udp[8:], []byte("ping"))
	pkt := buildIPv4(t, localA, localB, protoUDP, udp)
	assertValid(t, pkt, "before")

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused")
	}
	assertValid(t, pkt, "after")
}

// A UDP checksum of zero means "not computed" and has to stay that way.
func TestRewriteLeavesAnUncheckedUDPPacketAlone(t *testing.T) {
	udp := make([]byte, 8)
	binary.BigEndian.PutUint16(udp[4:6], 8)
	pkt := buildIPv4(t, localA, localB, protoUDP, udp)
	put16(pkt[20+6:20+8], 0)

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused")
	}
	if got := get16(pkt[20+6 : 20+8]); got != 0 {
		t.Fatalf("UDP checksum = %#04x, want it left at zero", got)
	}
	assertValid(t, pkt, "after")
}

func TestRewriteICMPEchoKeepsChecksumValid(t *testing.T) {
	icmp := make([]byte, 8+8)
	icmp[0] = 8 // echo request
	copy(icmp[8:], []byte("abcdefgh"))
	pkt := buildIPv4(t, localA, localB, protoICMP, icmp)
	assertValid(t, pkt, "before")

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused")
	}
	assertValid(t, pkt, "after")
}

// An ICMP error quotes the packet that caused it. The quote is how a host
// matches the error to a socket, so it has to be translated too.
func TestRewriteTranslatesTheHeaderQuotedInAnICMPError(t *testing.T) {
	inner := make([]byte, 20+8)
	inner[0] = 0x45
	binary.BigEndian.PutUint16(inner[2:4], uint16(len(inner)))
	inner[9] = protoUDP
	s, d := localB.As4(), localA.As4() // the quote runs the other way
	copy(inner[12:16], s[:])
	copy(inner[16:20], d[:])
	put16(inner[10:12], checksum(inner[:20]))

	icmp := make([]byte, 8+len(inner))
	icmp[0] = 3 // destination unreachable
	icmp[1] = 3 // port unreachable
	copy(icmp[8:], inner)

	pkt := buildIPv4(t, localA, localB, protoICMP, icmp)
	assertValid(t, pkt, "before")

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused")
	}
	assertValid(t, pkt, "after")

	quoted := pkt[20+8:]
	if addrAt(quoted[12:16]) != realB || addrAt(quoted[16:20]) != realA {
		t.Fatalf("quoted addresses = %v -> %v, want the translated pair",
			addrAt(quoted[12:16]), addrAt(quoted[16:20]))
	}
	if got := ones(quoted[:20]); got != 0xffff {
		t.Fatalf("quoted header checksum is wrong (sum %#04x)", got)
	}
}

// A packet for an address this network has no mapping for is dropped: forwarding
// it would put an address on the wire that means something else there.
func TestRewriteRefusesUnknownAddresses(t *testing.T) {
	pkt := buildIPv4(t, localA, netip.MustParseAddr("100.112.9.9"), protoUDP, make([]byte, 8))
	if rewrite(pkt, toReal()) {
		t.Fatal("rewrite accepted a packet for an unmapped address")
	}
}

func TestRewriteRejectsMalformedPackets(t *testing.T) {
	cases := map[string][]byte{
		"too short":      make([]byte, 10),
		"not IPv4":       append([]byte{0x60}, make([]byte, 39)...),
		"bad header len": append([]byte{0x40}, make([]byte, 39)...),
	}
	for name, pkt := range cases {
		if rewrite(pkt, toReal()) {
			t.Fatalf("rewrite accepted a %s packet", name)
		}
	}
}

// Only the first fragment carries a transport header; the rest must have their
// IP header fixed and nothing else touched.
func TestRewriteSkipsTransportFixupOnLaterFragments(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04}
	pkt := buildIPv4(t, localA, localB, protoUDP, payload)
	pkt[6] = 0x00
	pkt[7] = 0x10 // a non-zero fragment offset
	put16(pkt[10:12], 0)
	put16(pkt[10:12], checksum(pkt[:20]))
	before := append([]byte(nil), pkt[20:]...)

	if !rewrite(pkt, toReal()) {
		t.Fatal("rewrite refused a fragment")
	}
	if got := ones(pkt[:20]); got != 0xffff {
		t.Fatalf("fragment header checksum is wrong (sum %#04x)", got)
	}
	if string(pkt[20:]) != string(before) {
		t.Fatal("a later fragment's payload must not be treated as a transport header")
	}
}
