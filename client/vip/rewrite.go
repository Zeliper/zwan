package vip

import (
	"bytes"
	"encoding/binary"
	"net/netip"
)

const (
	protoICMP = 1
	protoTCP  = 6
	protoUDP  = 17
)

// mapFunc translates one address from the space a packet is leaving into the
// space it is entering.
type mapFunc func(netip.Addr) (netip.Addr, bool)

// rewrite translates every address in an IPv4 packet and repairs the checksums
// that cover them, reporting whether the packet can be forwarded.
//
// Every address in a packet is in one address space at a time — the operating
// system produces packets entirely in local addresses, and peers produce them
// entirely in overlay addresses — so one translation applies uniformly,
// including to a header quoted inside an ICMP error.
//
// An address with no mapping means a packet for a peer this network does not
// know, and it is dropped rather than forwarded with an address that means
// something else on the other side.
func rewrite(pkt []byte, translate mapFunc) bool {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return false
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl {
		return false
	}
	newSrc, srcOK := translate(addrAt(pkt[12:16]))
	newDst, dstOK := translate(addrAt(pkt[16:20]))
	if !srcOK || !dstOK {
		return false
	}
	var old, updated [8]byte
	copy(old[:], pkt[12:20])
	s4, d4 := newSrc.As4(), newDst.As4()
	copy(updated[:4], s4[:])
	copy(updated[4:], d4[:])
	if bytes.Equal(old[:], updated[:]) {
		return true
	}
	copy(pkt[12:20], updated[:])
	put16(pkt[10:12], csumUpdate(get16(pkt[10:12]), old[:], updated[:]))

	// Only the first fragment carries a transport header to repair.
	if pkt[6]&0x1f != 0 || pkt[7] != 0 {
		return true
	}
	l4 := pkt[ihl:]
	switch pkt[9] {
	case protoTCP:
		if len(l4) >= 18 {
			put16(l4[16:18], csumUpdate(get16(l4[16:18]), old[:], updated[:]))
		}
	case protoUDP:
		if len(l4) >= 8 {
			// A zero UDP checksum means "not computed" and must stay zero.
			if cs := get16(l4[6:8]); cs != 0 {
				fixed := csumUpdate(cs, old[:], updated[:])
				if fixed == 0 {
					fixed = 0xffff // UDP spells a zero checksum this way
				}
				put16(l4[6:8], fixed)
			}
		}
	case protoICMP:
		rewriteQuoted(l4, translate)
	}
	return true
}

// rewriteQuoted translates the header an ICMP error quotes back to the sender.
//
// The quote is how a host matches an error to the socket that caused it, so
// leaving it in the other address space is not cosmetic: it is how path MTU
// discovery and "connection refused" quietly stop working.
func rewriteQuoted(icmp []byte, translate mapFunc) {
	if len(icmp) < 8 {
		return
	}
	switch icmp[0] {
	case 3, 11, 12: // destination unreachable, time exceeded, parameter problem
	default:
		return
	}
	inner := icmp[8:]
	if len(inner) < 20 || inner[0]>>4 != 4 {
		return
	}
	newSrc, srcOK := translate(addrAt(inner[12:16]))
	newDst, dstOK := translate(addrAt(inner[16:20]))
	if !srcOK || !dstOK {
		return // best effort: an untranslatable quote is better than a corrupt one
	}
	var old, updated [8]byte
	copy(old[:], inner[12:20])
	s4, d4 := newSrc.As4(), newDst.As4()
	copy(updated[:4], s4[:])
	copy(updated[4:], d4[:])
	if bytes.Equal(old[:], updated[:]) {
		return
	}
	copy(inner[12:20], updated[:])

	icmpCS := csumUpdate(get16(icmp[2:4]), old[:], updated[:])
	// The quoted header carries its own checksum, and the ICMP checksum covers
	// the quote, so fixing one changes the input to the other.
	if innerIHL := int(inner[0]&0x0f) * 4; innerIHL >= 20 && len(inner) >= innerIHL {
		before := get16(inner[10:12])
		after := csumUpdate(before, old[:], updated[:])
		put16(inner[10:12], after)
		icmpCS = csumUpdate(icmpCS, u16(before), u16(after))
	}
	put16(icmp[2:4], icmpCS)
}

// csumUpdate applies RFC 1624 to a ones-complement checksum after part of the
// covered data changed, so a packet can be edited without summing all of it
// again. old and new must be the same even length.
func csumUpdate(cs uint16, old, updated []byte) uint16 {
	sum := uint32(^cs)
	for i := 0; i+1 < len(old); i += 2 {
		sum += uint32(^binary.BigEndian.Uint16(old[i:]))
		sum += uint32(binary.BigEndian.Uint16(updated[i:]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func addrAt(b []byte) netip.Addr { return netip.AddrFrom4([4]byte(b)) }

func get16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }

func put16(b []byte, v uint16) { binary.BigEndian.PutUint16(b, v) }

func u16(v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return b[:]
}
