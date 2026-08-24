package resolver

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestResolverAnswersAndNXDOMAIN(t *testing.T) {
	r := New("demo.zwan")
	r.SetRecords(map[string]net.IP{
		"minecraft.demo.zwan": net.ParseIP("100.64.0.2"),
		"alpha.demo.zwan":     net.ParseIP("100.64.0.1"),
	})
	addr, err := r.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = r.Serve() }()
	defer r.Shutdown()
	time.Sleep(100 * time.Millisecond)

	c := new(dns.Client)

	q := new(dns.Msg)
	q.SetQuestion("minecraft.demo.zwan.", dns.TypeA)
	resp, _, err := c.Exchange(q, addr)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("want 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "100.64.0.2" {
		t.Fatalf("unexpected answer: %v", resp.Answer[0])
	}

	q2 := new(dns.Msg)
	q2.SetQuestion("nope.demo.zwan.", dns.TypeA)
	resp2, _, err := c.Exchange(q2, addr)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Rcode != dns.RcodeNameError {
		t.Fatalf("want NXDOMAIN, got %s", dns.RcodeToString[resp2.Rcode])
	}
}

func TestResolverCaseInsensitive(t *testing.T) {
	r := New("Demo.Zwan")
	r.SetRecords(map[string]net.IP{"MineCraft.Demo.Zwan": net.ParseIP("100.64.0.9")})
	addr, err := r.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = r.Serve() }()
	defer r.Shutdown()
	time.Sleep(100 * time.Millisecond)

	q := new(dns.Msg)
	q.SetQuestion("minecraft.demo.zwan.", dns.TypeA)
	resp, _, err := new(dns.Client).Exchange(q, addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("want 1 answer, got %d", len(resp.Answer))
	}
}
