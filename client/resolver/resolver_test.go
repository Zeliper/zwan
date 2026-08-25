package resolver

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// serving starts a resolver on an ephemeral port and returns its address.
func serving(t *testing.T, r *Resolver) string {
	t.Helper()
	addr, err := r.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = r.Serve() }()
	t.Cleanup(func() { _ = r.Shutdown() })
	time.Sleep(100 * time.Millisecond)
	return addr
}

// ask sends one A query and returns the answer address and the response code.
func ask(t *testing.T, addr, name string) (string, int) {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := new(dns.Client).Exchange(q, addr)
	if err != nil {
		t.Fatalf("exchange %s: %v", name, err)
	}
	if len(resp.Answer) == 0 {
		return "", resp.Rcode
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("unexpected answer type %T", resp.Answer[0])
	}
	return a.A.String(), resp.Rcode
}

func TestResolverAnswersAndNXDOMAIN(t *testing.T) {
	r := New()
	r.SetZone("demo.zwan", map[string]net.IP{
		"minecraft.demo.zwan": net.ParseIP("100.64.0.2"),
		"alpha.demo.zwan":     net.ParseIP("100.64.0.1"),
	})
	addr := serving(t, r)

	if ip, rcode := ask(t, addr, "minecraft.demo.zwan"); ip != "100.64.0.2" || rcode != dns.RcodeSuccess {
		t.Fatalf("= (%q, %s)", ip, dns.RcodeToString[rcode])
	}
	if _, rcode := ask(t, addr, "nope.demo.zwan"); rcode != dns.RcodeNameError {
		t.Fatalf("want NXDOMAIN inside a joined zone, got %s", dns.RcodeToString[rcode])
	}
}

func TestResolverCaseInsensitive(t *testing.T) {
	r := New()
	r.SetZone("Demo.Zwan", map[string]net.IP{"MineCraft.Demo.Zwan": net.ParseIP("100.64.0.9")})
	addr := serving(t, r)

	if ip, _ := ask(t, addr, "minecraft.demo.zwan"); ip != "100.64.0.9" {
		t.Fatalf("= %q", ip)
	}
}

// Several networks answer from one listener: a device can only own :53 once.
func TestResolverServesSeveralZonesAtOnce(t *testing.T) {
	r := New()
	r.SetZone("alice", map[string]net.IP{"nas.alice": net.ParseIP("100.64.0.5")})
	r.SetZone("bob", map[string]net.IP{"nas.bob": net.ParseIP("100.77.0.5")})
	addr := serving(t, r)

	if ip, _ := ask(t, addr, "nas.alice"); ip != "100.64.0.5" {
		t.Fatalf("nas.alice = %q", ip)
	}
	if ip, _ := ask(t, addr, "nas.bob"); ip != "100.77.0.5" {
		t.Fatalf("nas.bob = %q", ip)
	}
	// The same short name in two networks stays distinct because the suffix does.
	if want := []string{"alice", "bob"}; !reflect.DeepEqual(r.Suffixes(), want) {
		t.Fatalf("suffixes = %v, want %v", r.Suffixes(), want)
	}
}

// Leaving a network must stop its names resolving immediately.
func TestRemoveZoneStopsAnswering(t *testing.T) {
	r := New()
	r.SetZone("alice", map[string]net.IP{"nas.alice": net.ParseIP("100.64.0.5")})
	r.SetZone("bob", map[string]net.IP{"nas.bob": net.ParseIP("100.77.0.5")})
	addr := serving(t, r)

	r.RemoveZone("alice")
	if _, rcode := ask(t, addr, "nas.alice"); rcode != dns.RcodeRefused {
		t.Fatalf("a name in no joined zone should be refused, got %s", dns.RcodeToString[rcode])
	}
	if ip, _ := ask(t, addr, "nas.bob"); ip != "100.77.0.5" {
		t.Fatalf("the remaining zone should still answer, got %q", ip)
	}
}

// A query that reaches us for a suffix we do not serve was misrouted; refusing
// it is honest, answering NXDOMAIN would claim the name does not exist anywhere.
func TestUnknownSuffixIsRefused(t *testing.T) {
	r := New()
	r.SetZone("alice", map[string]net.IP{"nas.alice": net.ParseIP("100.64.0.5")})
	addr := serving(t, r)

	if _, rcode := ask(t, addr, "example.com"); rcode != dns.RcodeRefused {
		t.Fatalf("want REFUSED, got %s", dns.RcodeToString[rcode])
	}
}

// A zone whose name is the record itself (the network's own apex) resolves.
func TestApexNameResolves(t *testing.T) {
	r := New()
	r.SetZone("alice", map[string]net.IP{"alice": net.ParseIP("100.64.0.1")})
	addr := serving(t, r)

	if ip, _ := ask(t, addr, "alice"); ip != "100.64.0.1" {
		t.Fatalf("= %q", ip)
	}
}
