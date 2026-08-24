package ipam

import (
	"strconv"
	"testing"
)

func TestAllocateStableAndDistinct(t *testing.T) {
	a, err := New("100.64.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	ip1, err := a.Allocate("dev-1")
	if err != nil {
		t.Fatal(err)
	}
	ip1again, err := a.Allocate("dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != ip1again {
		t.Fatalf("address not stable for same device: %v vs %v", ip1, ip1again)
	}
	ip2, err := a.Allocate("dev-2")
	if err != nil {
		t.Fatal(err)
	}
	if ip1 == ip2 {
		t.Fatal("distinct devices received the same address")
	}
}

func TestSkipsNetworkAddress(t *testing.T) {
	a, err := New("100.64.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	ip, err := a.Allocate("dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "100.64.0.1" {
		t.Fatalf("first allocation should be .1, got %s", ip)
	}
}

func TestExhaustion(t *testing.T) {
	// /30 => .0-.3 in prefix; allocation starts at .1 => at most 3 addresses.
	a, err := New("100.64.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		ip, err := a.Allocate("k" + strconv.Itoa(i))
		if err != nil {
			break // exhausted, as expected
		}
		seen[ip.String()] = true
	}
	if len(seen) == 0 || len(seen) > 3 {
		t.Fatalf("expected 1..3 allocations in /30, got %d", len(seen))
	}
}
