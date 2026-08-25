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

// Nodes and services come from separate halves, so a service address can never
// be handed to a device.
func TestSplitSeparatesNodesFromServices(t *testing.T) {
	nodes, services, err := Split("100.64.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	if got := nodes.Prefix().String(); got != "100.64.0.0/17" {
		t.Fatalf("node range = %s", got)
	}
	if got := services.Prefix().String(); got != "100.64.128.0/17" {
		t.Fatalf("service range = %s", got)
	}

	node, err := nodes.Allocate("device-1")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := services.Allocate("minecraft")
	if err != nil {
		t.Fatal(err)
	}
	if !nodes.Prefix().Contains(node) || nodes.Prefix().Contains(svc) {
		t.Fatalf("node %v and service %v are not in their own halves", node, svc)
	}
	if !services.Prefix().Contains(svc) {
		t.Fatalf("service %v is outside the service range", svc)
	}
	// Devices still start where they always did, so existing networks see no
	// change in the addresses they hand out.
	if node.String() != "100.64.0.1" {
		t.Fatalf("first device address = %s, want the range to start where it used to", node)
	}
}

func TestSplitIsStablePerKey(t *testing.T) {
	_, services, err := Split("100.64.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := services.Allocate("nas")
	again, _ := services.Allocate("nas")
	other, _ := services.Allocate("git")
	if first != again {
		t.Fatalf("a name moved: %v then %v", first, again)
	}
	if other == first {
		t.Fatal("two names got the same address")
	}
}

func TestSplitRejectsRangesWithNoRoom(t *testing.T) {
	if _, _, err := Split("100.64.0.0/30"); err == nil {
		t.Fatal("a range too small to hold both halves should be refused")
	}
	if _, _, err := Split("not-a-cidr"); err == nil {
		t.Fatal("a malformed range should be refused")
	}
}
