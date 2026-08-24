package join

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

const stubNodeToken = "stub-node-token"

// controlStub serves just enough of the control API to exercise the client. It
// records the Authorization header of the last /v1/peers call so tests can check
// that the node token is actually being sent.
func controlStub(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastAuth string
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
			NodeToken: stubNodeToken,
		})
	})
	mux.HandleFunc("/v1/peers", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(proto.PeersResponse{Peers: []proto.Peer{{Hostname: "alice", AssignedIP: "100.64.0.1"}}})
	})
	mux.HandleFunc("/v1/services", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(proto.ServicesResponse{Services: []proto.Service{{Name: "minecraft", Proto: "tcp", Port: 25565, NodeIP: "100.64.0.1"}}})
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth
}

func TestPinnedClientTalksToASelfSignedServer(t *testing.T) {
	srv, lastAuth := controlStub(t)
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

	// The join token must not be reused: later calls carry the node token the
	// server issued at registration.
	if !cl.Authenticated() {
		t.Fatal("client did not keep the node token")
	}
	if want := "Bearer " + stubNodeToken; *lastAuth != want {
		t.Fatalf("Authorization = %q, want %q", *lastAuth, want)
	}
}

func TestJoinRejectsAServerThatIssuesNoNodeToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(proto.RegisterResponse{NetworkID: "demo", AssignedIP: "100.64.0.7"})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cl, err := NewClient(srv.URL, certpin.OfCert(srv.Certificate()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Join("token", "device", "bob", ""); err == nil {
		t.Fatal("join should fail when the server issues no node token")
	}
}

func TestPinCanTravelInsideTheServerURL(t *testing.T) {
	srv, _ := controlStub(t)
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
	srv, _ := controlStub(t)

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

// restartableStub models a control server whose in-memory state can be wiped,
// which is what a server restart looks like to a client holding a node token.
type restartableStub struct {
	mu       sync.Mutex
	token    string
	issued   int
	assigned string
	pubKeys  []string
}

func (st *restartableStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", func(w http.ResponseWriter, r *http.Request) {
		var req proto.RegisterRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		st.mu.Lock()
		st.issued++
		st.token = fmt.Sprintf("node-token-%d", st.issued)
		st.pubKeys = append(st.pubKeys, req.PublicKey)
		resp := proto.RegisterResponse{
			NetworkID: "demo", DNSSuffix: "demo.zwan", OverlayCIDR: "100.64.0.0/16",
			AssignedIP: st.assigned, NodeToken: st.token,
		}
		st.mu.Unlock()
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/v1/peers", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		want := "Bearer " + st.token
		st.mu.Unlock()
		if st.token == "" || r.Header.Get("Authorization") != want {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(proto.ErrorResponse{Error: "a node token is required; register first"})
			return
		}
		_ = json.NewEncoder(w).Encode(proto.PeersResponse{Peers: []proto.Peer{{Hostname: "alice", AssignedIP: "100.64.0.1"}}})
	})
	return mux
}

// forget wipes the issued token, as a restart would.
func (st *restartableStub) forget() {
	st.mu.Lock()
	st.token = ""
	st.mu.Unlock()
}

func TestClientReRegistersAfterTheServerForgetsIt(t *testing.T) {
	st := &restartableStub{assigned: "100.64.0.7"}
	srv := httptest.NewTLSServer(st.handler())
	defer srv.Close()

	cl, err := NewClient(srv.URL, certpin.OfCert(srv.Certificate()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Join("token", "device", "bob", ""); err != nil {
		t.Fatal(err)
	}

	st.forget()
	if _, err := cl.Peers(); err != nil {
		t.Fatalf("client did not recover from a server restart: %v", err)
	}
	if st.issued != 2 {
		t.Fatalf("expected exactly one re-registration, got %d registrations", st.issued)
	}
	// The node key is this device's identity on the tunnel; refreshing a token
	// must not change it, or every peer's WireGuard config goes stale.
	if st.pubKeys[0] != st.pubKeys[1] {
		t.Fatalf("re-registration changed the node key: %q -> %q", st.pubKeys[0], st.pubKeys[1])
	}
}

func TestReRegistrationReportsAChangedOverlayAddress(t *testing.T) {
	st := &restartableStub{assigned: "100.64.0.7"}
	srv := httptest.NewTLSServer(st.handler())
	defer srv.Close()

	cl, err := NewClient(srv.URL, certpin.OfCert(srv.Certificate()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Join("token", "device", "bob", ""); err != nil {
		t.Fatal(err)
	}

	st.forget()
	st.mu.Lock()
	st.assigned = "100.64.0.9" // the restarted server allocated a different address
	st.mu.Unlock()

	_, err = cl.Peers()
	if err == nil {
		t.Fatal("a reassigned overlay address should be reported, not used silently")
	}
	if !strings.Contains(err.Error(), "reassigned") {
		t.Fatalf("unexpected error: %v", err)
	}
}
