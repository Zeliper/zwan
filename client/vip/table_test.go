package vip

import (
	"net/netip"
	"testing"
)

func mustTable(t *testing.T, prefix string) *Table {
	t.Helper()
	return mustTableFor(t, prefix, "100.64.0.0/16")
}

func mustTableFor(t *testing.T, local, real string) *Table {
	t.Helper()
	tbl, err := NewTable(netip.MustParsePrefix(local), netip.MustParsePrefix(real))
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

// Keeping the low bits is only a readability convenience, but it is the case
// that matters: 100.64.0.7 in a network mapped to 100.112.0.0/16 stays .0.7.
func TestLocalKeepsTheLowBitsWhenItCan(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/16")

	got, ok := tbl.Local(netip.MustParseAddr("100.64.0.7"))
	if !ok || got != netip.MustParseAddr("100.112.0.7") {
		t.Fatalf("= (%v, %v)", got, ok)
	}
	got, ok = tbl.Local(netip.MustParseAddr("100.64.3.200"))
	if !ok || got != netip.MustParseAddr("100.112.3.200") {
		t.Fatalf("= (%v, %v)", got, ok)
	}
}

func TestMappingIsStableAndTwoWay(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/16")
	real := netip.MustParseAddr("100.64.0.9")

	first, _ := tbl.Local(real)
	again, _ := tbl.Local(real)
	if first != again {
		t.Fatalf("a repeated lookup moved: %v then %v", first, again)
	}
	back, ok := tbl.Real(first)
	if !ok || back != real {
		t.Fatalf("reverse lookup = (%v, %v), want %v", back, ok, real)
	}
	if _, ok := tbl.Real(netip.MustParseAddr("100.112.9.9")); ok {
		t.Fatal("an address that was never handed out must not reverse-map")
	}
}

// Two real addresses can want the same low bits when the real range is larger
// than the local one. The second gets a different address rather than colliding.
func TestCollidingLowBitsGetDistinctAddresses(t *testing.T) {
	tbl := mustTableFor(t, "100.112.0.0/24", "100.64.0.0/16") // only 8 local host bits

	a, okA := tbl.Local(netip.MustParseAddr("100.64.0.5"))
	b, okB := tbl.Local(netip.MustParseAddr("100.64.1.5")) // same low 8 bits
	if !okA || !okB {
		t.Fatal("both addresses should be mappable")
	}
	if a == b {
		t.Fatalf("both real addresses mapped to %v", a)
	}
	if back, _ := tbl.Real(a); back != netip.MustParseAddr("100.64.0.5") {
		t.Fatalf("reverse of %v = %v", a, back)
	}
	if back, _ := tbl.Real(b); back != netip.MustParseAddr("100.64.1.5") {
		t.Fatalf("reverse of %v = %v", b, back)
	}
}

// The network and broadcast addresses are not usable as host addresses.
func TestNetworkAndBroadcastAreNeverHandedOut(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/30") // .0 network, .1 .2 hosts, .3 broadcast

	seen := map[netip.Addr]bool{}
	for i := 0; i < 4; i++ {
		got, ok := tbl.Local(netip.AddrFrom4([4]byte{100, 64, 0, byte(i)}))
		if !ok {
			continue
		}
		seen[got] = true
	}
	for _, bad := range []string{"100.112.0.0", "100.112.0.3"} {
		if seen[netip.MustParseAddr(bad)] {
			t.Fatalf("%s was handed out", bad)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("a /30 has two usable addresses, got %d: %v", len(seen), seen)
	}
}

// A full range reports failure rather than handing out a duplicate.
func TestExhaustedRangeFails(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/30")
	_, _ = tbl.Local(netip.MustParseAddr("100.64.0.1"))
	_, _ = tbl.Local(netip.MustParseAddr("100.64.0.2"))

	if _, ok := tbl.Local(netip.MustParseAddr("100.64.0.9")); ok {
		t.Fatal("an exhausted range must not hand out another address")
	}
}

func TestNewTableRejectsUnusableRanges(t *testing.T) {
	overlay := netip.MustParsePrefix("100.64.0.0/16")
	if _, err := NewTable(netip.MustParsePrefix("100.112.0.0/31"), overlay); err == nil {
		t.Fatal("a range with no usable host addresses should be refused")
	}
	if _, err := NewTable(netip.MustParsePrefix("fd00::/64"), overlay); err == nil {
		t.Fatal("the overlay is IPv4; an IPv6 local range should be refused")
	}
	if _, err := NewTable(netip.MustParsePrefix("100.112.0.0/16"), netip.MustParsePrefix("fd00::/64")); err == nil {
		t.Fatal("an IPv6 overlay range should be refused")
	}
}

func TestPrefixIsMasked(t *testing.T) {
	tbl := mustTable(t, "100.112.5.7/16")
	if got := tbl.Prefix(); got != netip.MustParsePrefix("100.112.0.0/16") {
		t.Fatalf("Prefix() = %v", got)
	}
}

// The table only maps its own network's addresses, so a peer cannot make traffic
// appear to come from somewhere else in the local space.
func TestAddressesOutsideTheOverlayAreNotMapped(t *testing.T) {
	tbl := mustTable(t, "100.112.0.0/16")
	for _, outside := range []string{"10.0.0.1", "192.168.1.5", "100.65.0.1"} {
		if _, ok := tbl.Local(netip.MustParseAddr(outside)); ok {
			t.Fatalf("%s is outside the overlay range but was mapped", outside)
		}
	}
}
