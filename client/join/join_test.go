package join

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Zeliper/zwan/shared/certpin"
	"github.com/Zeliper/zwan/shared/proto"
)

func TestSplitPin(t *testing.T) {
	cases := []struct{ in, wantServer, wantPin string }{
		{"https://example.test:8787", "https://example.test:8787", ""},
		{"https://example.test:8787#sha256:AAAA", "https://example.test:8787", "sha256:AAAA"},
		{" https://1.2.3.4:443#sha256:a%2Fb+c= ", "https://1.2.3.4:443", "sha256:a/b+c="},
		{"", "", ""},
	}
	for _, c := range cases {
		server, pin := SplitPin(c.in)
		if server != c.wantServer || pin != c.wantPin {
			t.Fatalf("SplitPin(%q) = (%q, %q), want (%q, %q)", c.in, server, pin, c.wantServer, c.wantPin)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	ok := map[string]string{
		"example.test:8787":              "https://example.test:8787",
		"https://example.test:8787/":     "https://example.test:8787",
		"https://example.test:8787/v1/x": "https://example.test:8787",
		"http://127.0.0.1:8787":          "http://127.0.0.1:8787",
	}
	for in, want := range ok {
		got, err := NormalizeURL(in)
		if err != nil {
			t.Fatalf("NormalizeURL(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://example.test", "https://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Fatalf("NormalizeURL(%q) accepted a bad address", bad)
		}
	}
}

// controlStub serves just enough of the control API to exercise the client.
func controlStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", func(w http.ResponseWriter, r *http.Request) {
		var req proto.RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if req.PublicKey == "" {
			http.Error(w, "no key", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(proto.RegisterResponse{
			NetworkID: "demo", DNSSuffix: "demo.zwan", OverlayCIDR: "100.64.0.0/16", AssignedIP: "100.64.0.7",
		})
	})
	mux.HandleFunc("/v1/peers", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(proto.PeersResponse{Peers: []proto.Peer{{Hostname: "alice", AssignedIP: "100.64.0.1"}}})
	})
	mux.HandleFunc("/v1/services", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(proto.ServicesResponse{Services: []proto.Service{{Name: "minecraft", Proto: "tcp", Port: 25565, NodeIP: "100.64.0.1"}}})
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestPinnedClientTalksToASelfSignedServer(t *testing.T) {
	srv := controlStub(t)
	pin := certpin.OfCert(srv.Certificate())

	cl, err := NewClient(srv.URL, pin)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cl.Join("token", "device", "bob", "127.0.0.1:51820")
	if err != nil {
		t.Fatalf("join over a pinned connection: %v", err)
	}
	if res.Register.AssignedIP != "100.64.0.7" {
		t.Fatalf("assigned IP = %q", res.Register.AssignedIP)
	}
	if len(res.Peers) != 1 || res.Peers[0].Hostname != "alice" {
		t.Fatalf("peers = %+v", res.Peers)
	}
	svcs, err := cl.Services()
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 1 || svcs[0].Name != "minecraft" {
		t.Fatalf("services = %+v", svcs)
	}
}

func TestPinCanTravelInsideTheServerURL(t *testing.T) {
	srv := controlStub(t)
	pin := certpin.OfCert(srv.Certificate())

	cl, err := NewClient(srv.URL+"#"+url.PathEscape(pin), "")
	if err != nil {
		t.Fatal(err)
	}
	if cl.Pin() != pin {
		t.Fatalf("pin = %q, want %q", cl.Pin(), pin)
	}
	if cl.BaseURL() != srv.URL {
		t.Fatalf("base URL = %q, want %q", cl.BaseURL(), srv.URL)
	}
	if _, err := cl.Peers(); err != nil {
		t.Fatalf("peers over a URL-embedded pin: %v", err)
	}
}

func TestWrongOrMissingPinIsRejected(t *testing.T) {
	srv := controlStub(t)

	// A syntactically valid pin for a different key must not connect.
	other := certpin.OfSPKI([]byte("some other key"))
	cl, err := NewClient(srv.URL, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Peers(); err == nil {
		t.Fatal("client accepted a server whose key does not match the pin")
	} else if !strings.Contains(err.Error(), "does not match the expected pin") {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no pin the self-signed server is untrusted by the system store.
	unpinned, err := NewClient(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unpinned.Peers(); err == nil {
		t.Fatal("client accepted an untrusted certificate with no pin")
	}
}

func TestNewClientRejectsAMalformedPin(t *testing.T) {
	if _, err := NewClient("https://example.test:8787", "not-a-fingerprint"); err == nil {
		t.Fatal("NewClient accepted a malformed pin")
	}
}
