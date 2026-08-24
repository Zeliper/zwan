package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/store"
	"github.com/Zeliper/zwan/shared/proto"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	alloc, err := ipam.New("100.64.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	nw := store.NewNetwork("demo", "demo.zwan", "100.64.0.0/16")
	ts := httptest.NewServer(New(nw, alloc, "secret", "127.0.0.1:3478").Routes())
	t.Cleanup(ts.Close)
	return ts
}

func register(t *testing.T, base string, req proto.RegisterRequest) (int, proto.RegisterResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	resp, err := http.Post(base+"/v1/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out proto.RegisterResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// do issues a request as a member; an empty token sends no Authorization header.
func do(t *testing.T, method, url, token string, body any) (int, []byte) {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func TestRegisterInvalidToken(t *testing.T) {
	ts := newTestServer(t)
	code, _ := register(t, ts.URL, proto.RegisterRequest{Token: "wrong", DeviceUUID: "d1", PublicKey: "k1"})
	if code != http.StatusUnauthorized {
		t.Fatalf("want 401 for bad token, got %d", code)
	}
}

func TestRegisterRequiresFields(t *testing.T) {
	ts := newTestServer(t)
	code, _ := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "", PublicKey: ""})
	if code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing fields, got %d", code)
	}
}

func TestRegisterAllocatesStableIPAndListsPeers(t *testing.T) {
	ts := newTestServer(t)

	code, r1 := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", Hostname: "alpha", PublicKey: "k1"})
	if code != http.StatusOK || r1.AssignedIP == "" {
		t.Fatalf("register d1 failed: code=%d ip=%q", code, r1.AssignedIP)
	}
	if r1.NetworkID != "demo" || r1.DNSSuffix != "demo.zwan" {
		t.Fatalf("unexpected network metadata: %+v", r1)
	}

	_, r1b := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", Hostname: "alpha", PublicKey: "k1"})
	if r1b.AssignedIP != r1.AssignedIP {
		t.Fatalf("re-register should keep the same IP: %s vs %s", r1.AssignedIP, r1b.AssignedIP)
	}

	_, r2 := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d2", Hostname: "beta", PublicKey: "k2"})
	if r2.AssignedIP == r1.AssignedIP {
		t.Fatalf("distinct devices must get distinct IPs, both got %s", r1.AssignedIP)
	}

	code, body := do(t, http.MethodGet, ts.URL+"/v1/peers", r2.NodeToken, nil)
	if code != http.StatusOK {
		t.Fatalf("peers: want 200, got %d", code)
	}
	var pr proto.PeersResponse
	_ = json.Unmarshal(body, &pr)
	if len(pr.Peers) != 2 {
		t.Fatalf("want 2 peers, got %d", len(pr.Peers))
	}
}

// Registration hands out a per-device credential; the join token is not one.
func TestRegisterIssuesADistinctNodeTokenPerDevice(t *testing.T) {
	ts := newTestServer(t)

	_, r1 := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})
	_, r2 := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d2", PublicKey: "k2"})
	if r1.NodeToken == "" || r2.NodeToken == "" {
		t.Fatal("register must return a node token")
	}
	if r1.NodeToken == r2.NodeToken {
		t.Fatal("two devices got the same node token")
	}
	if r1.NodeToken == "secret" {
		t.Fatal("the node token must not be the join token")
	}
}

// Re-registering a device rotates its credential, so the old one stops working.
func TestReRegisterInvalidatesThePreviousNodeToken(t *testing.T) {
	ts := newTestServer(t)

	_, first := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})
	_, second := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})
	if first.NodeToken == second.NodeToken {
		t.Fatal("re-registration should issue a fresh node token")
	}
	if code, _ := do(t, http.MethodGet, ts.URL+"/v1/peers", first.NodeToken, nil); code != http.StatusUnauthorized {
		t.Fatalf("stale node token: want 401, got %d", code)
	}
	if code, _ := do(t, http.MethodGet, ts.URL+"/v1/peers", second.NodeToken, nil); code != http.StatusOK {
		t.Fatalf("current node token: want 200, got %d", code)
	}
}

// The membership and service directories are not public: an outsider who can
// reach the API learns nothing about the network without a node token.
func TestDirectoriesRequireANodeToken(t *testing.T) {
	ts := newTestServer(t)
	register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})

	for _, url := range []string{ts.URL + "/v1/peers", ts.URL + "/v1/services"} {
		if code, _ := do(t, http.MethodGet, url, "", nil); code != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated: want 401, got %d", url, code)
		}
		if code, _ := do(t, http.MethodGet, url, "secret", nil); code != http.StatusUnauthorized {
			t.Fatalf("%s with the join token: want 401, got %d", url, code)
		}
		if code, _ := do(t, http.MethodGet, url, "not-a-token", nil); code != http.StatusUnauthorized {
			t.Fatalf("%s with a bogus token: want 401, got %d", url, code)
		}
	}
}

func TestServicesRegisterAndList(t *testing.T) {
	ts := newTestServer(t)
	_, me := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})

	svc := proto.Service{Name: "minecraft", Proto: "tcp", Port: 25565, NodeIP: me.AssignedIP}
	if code, body := do(t, http.MethodPost, ts.URL+"/v1/services", me.NodeToken, proto.RegisterServiceRequest{Service: svc}); code != http.StatusOK {
		t.Fatalf("publish service: want 200, got %d (%s)", code, body)
	}

	code, body := do(t, http.MethodGet, ts.URL+"/v1/services", me.NodeToken, nil)
	if code != http.StatusOK {
		t.Fatalf("list services: want 200, got %d", code)
	}
	var sr proto.ServicesResponse
	_ = json.Unmarshal(body, &sr)
	if len(sr.Services) != 1 || sr.Services[0].Name != "minecraft" || sr.Services[0].NodeIP != me.AssignedIP || sr.Services[0].Port != 25565 {
		t.Fatalf("unexpected services: %+v", sr.Services)
	}
}

// Omitting node_ip means "on me", which is the only node a member may claim.
func TestServiceDefaultsToTheCallersNode(t *testing.T) {
	ts := newTestServer(t)
	_, me := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})

	req := proto.RegisterServiceRequest{Service: proto.Service{Name: "nas", Port: 445}}
	code, body := do(t, http.MethodPost, ts.URL+"/v1/services", me.NodeToken, req)
	if code != http.StatusOK {
		t.Fatalf("publish: want 200, got %d (%s)", code, body)
	}
	var out proto.Service
	_ = json.Unmarshal(body, &out)
	if out.NodeIP != me.AssignedIP {
		t.Fatalf("node IP = %q, want the caller's %q", out.NodeIP, me.AssignedIP)
	}
	if out.Proto != "tcp" {
		t.Fatalf("proto = %q, want the tcp default", out.Proto)
	}
}

// A member must not be able to point a name at somebody else's node.
func TestServiceCannotBePublishedOnAnotherNode(t *testing.T) {
	ts := newTestServer(t)
	_, a := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})
	_, b := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d2", PublicKey: "k2"})

	req := proto.RegisterServiceRequest{Service: proto.Service{Name: "spoof", Proto: "tcp", Port: 25565, NodeIP: b.AssignedIP}}
	if code, _ := do(t, http.MethodPost, ts.URL+"/v1/services", a.NodeToken, req); code != http.StatusForbidden {
		t.Fatalf("cross-node publish: want 403, got %d", code)
	}
}

func TestPublishRequiresANodeToken(t *testing.T) {
	ts := newTestServer(t)
	register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "d1", PublicKey: "k1"})

	req := proto.RegisterServiceRequest{Service: proto.Service{Name: "x", Proto: "tcp", Port: 1}}
	if code, _ := do(t, http.MethodPost, ts.URL+"/v1/services", "", req); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated publish: want 401, got %d", code)
	}
	if code, _ := do(t, http.MethodPost, ts.URL+"/v1/services", "secret", req); code != http.StatusUnauthorized {
		t.Fatalf("publish with the join token: want 401, got %d", code)
	}
}

// Liveness stays open so an operator or load balancer can probe it.
func TestHealthzIsPublic(t *testing.T) {
	ts := newTestServer(t)
	if code, _ := do(t, http.MethodGet, ts.URL+"/healthz", "", nil); code != http.StatusOK {
		t.Fatalf("healthz: want 200, got %d", code)
	}
}
