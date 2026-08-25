package host

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Zeliper/zwan/client/join"
)

func startTestHost(t *testing.T, cfg Config) *Host {
	t.Helper()
	cfg.NetworkID = "demo"
	cfg.DNSSuffix = "demo.zwan"
	cfg.CIDR = "100.64.0.0/24"
	cfg.Token = "test-token"
	cfg.ControlAddr = "127.0.0.1:0"
	cfg.RelayAddr = "127.0.0.1:0"
	if cfg.TLSDir == "" && cfg.TLSMode != "off" {
		cfg.TLSDir = t.TempDir()
	}
	h := New()
	if err := h.Start(cfg); err != nil {
		t.Fatalf("start host: %v", err)
	}
	t.Cleanup(h.Stop)
	return h
}

// The default (no domain configured) must be a pinned TLS server, not plaintext.
func TestDefaultHostIsPinnedTLS(t *testing.T) {
	h := startTestHost(t, Config{})

	if h.TLSMode() != "self" {
		t.Fatalf("TLS mode = %q, want self", h.TLSMode())
	}
	if h.Scheme() != "https" {
		t.Fatalf("scheme = %q, want https", h.Scheme())
	}
	if h.Pin() == "" {
		t.Fatal("a self-signed host must publish a pin")
	}
	if !strings.HasPrefix(h.LocalURL(), "https://127.0.0.1:") {
		t.Fatalf("local URL = %q", h.LocalURL())
	}
	if strings.HasSuffix(h.LocalURL(), ":0") {
		t.Fatalf("local URL kept the unresolved port: %q", h.LocalURL())
	}

	cl, err := join.NewClient(h.LocalURL(), h.Pin())
	if err != nil {
		t.Fatal(err)
	}
	res, err := cl.Join("test-token", "device-1", "alice", "127.0.0.1:51820")
	if err != nil {
		t.Fatalf("join over TLS: %v", err)
	}
	if res.Register.NetworkID != "demo" || res.Register.AssignedIP == "" {
		t.Fatalf("register response = %+v", res.Register)
	}

	// The host reads its own store, so the GUI sees members without holding a
	// member credential of its own.
	members := h.Members()
	if len(members) != 1 || members[0].Hostname != "alice" {
		t.Fatalf("host members = %+v", members)
	}
}

// The printed join address carries the pin, so it can be copied as one value.
func TestJoinURLCarriesThePin(t *testing.T) {
	h := startTestHost(t, Config{})

	server, pin := join.SplitPin(h.JoinURL(""))
	if pin != h.Pin() {
		t.Fatalf("join URL pin = %q, want %q", pin, h.Pin())
	}
	cl, err := join.NewClient(h.JoinURL(""), "")
	if err != nil {
		t.Fatal(err)
	}
	if cl.BaseURL() != server {
		t.Fatalf("base URL = %q, want %q", cl.BaseURL(), server)
	}
	if _, err := cl.Join("test-token", "device-1", "alice", ""); err != nil {
		t.Fatalf("join via the join URL: %v", err)
	}
	if _, err := cl.Peers(); err != nil {
		t.Fatalf("peers via the join URL: %v", err)
	}

	if got := h.JoinURL("vpn.example.test:8787"); !strings.HasPrefix(got, "https://vpn.example.test:8787#") {
		t.Fatalf("join URL with an explicit public host = %q", got)
	}
}

// A public address without a port has to pick up the port the server listens on.
//
// Left alone it becomes an address the client can still parse and still reach —
// on 443, where whatever else is published on that address answers. The pin then
// fails against a stranger's key, which reads as "the pin is wrong" rather than
// "the port is missing" and sends the operator looking in the wrong place.
func TestJoinURLGivesAPortlessPublicAddressTheListeningPort(t *testing.T) {
	h := startTestHost(t, Config{})

	local, err := url.Parse(h.LocalURL())
	if err != nil {
		t.Fatal(err)
	}
	port := local.Port()
	want := "https://" + net.JoinHostPort("203.0.113.5", port) + "#"
	if got := h.JoinURL("203.0.113.5"); !strings.HasPrefix(got, want) {
		t.Errorf("JoinURL(%q) = %q, want it to start with %q", "203.0.113.5", got, want)
	}

	// One that already names a port is the operator's business, port 443
	// included: they may be behind something that forwards it.
	for _, given := range []string{"203.0.113.5:9999", "vpn.example.test:443", "[2001:db8::1]:8787"} {
		if got := h.JoinURL(given); !strings.HasPrefix(got, "https://"+given+"#") {
			t.Errorf("JoinURL(%q) = %q, want it left alone", given, got)
		}
	}

	// An unbracketed IPv6 literal names no port, and has to come back usable.
	if got := h.JoinURL("2001:db8::1"); !strings.HasPrefix(got, "https://[2001:db8::1]:"+port+"#") {
		t.Errorf("JoinURL of a bare IPv6 address = %q", got)
	}
}

// An untrusted client must not be able to reach a pinned host.
func TestPlainHTTPClientCannotReachTheTLSHost(t *testing.T) {
	h := startTestHost(t, Config{})

	insecure := strings.Replace(h.LocalURL(), "https://", "http://", 1)
	resp, err := http.Get(insecure + "/healthz")
	if err != nil {
		return // the transport refused it outright, which is also fine
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("the control API answered a plaintext request on a TLS listener")
	}
}

// --tls=off stays available for local development and reverse-proxy setups.
func TestTLSOffServesPlaintext(t *testing.T) {
	h := startTestHost(t, Config{TLSMode: "off"})

	if h.TLSMode() != "off" || h.Scheme() != "http" {
		t.Fatalf("mode = %q, scheme = %q", h.TLSMode(), h.Scheme())
	}
	if h.Pin() != "" {
		t.Fatal("plaintext mode must not advertise a pin")
	}
	cl, err := join.NewClient(h.LocalURL(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Join("test-token", "device-1", "alice", ""); err != nil {
		t.Fatalf("join over plaintext: %v", err)
	}
	if _, err := cl.Peers(); err != nil {
		t.Fatalf("peers over plaintext: %v", err)
	}
}

func TestStartTwiceFails(t *testing.T) {
	h := startTestHost(t, Config{})
	if err := h.Start(Config{ControlAddr: "127.0.0.1:0", RelayAddr: "127.0.0.1:0", CIDR: "100.64.0.0/24"}); err == nil {
		t.Fatal("starting an already-running host should fail")
	}
}

func TestUnknownTLSModeIsRejected(t *testing.T) {
	h := New()
	err := h.Start(Config{
		NetworkID: "demo", CIDR: "100.64.0.0/24", Token: "t",
		ControlAddr: "127.0.0.1:0", RelayAddr: "127.0.0.1:0", TLSMode: "mtls",
	})
	if err == nil {
		h.Stop()
		t.Fatal("an unknown TLS mode should be rejected")
	}
}
