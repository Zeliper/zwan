package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/store"
	"github.com/Zeliper/zwan/shared/acl"
	"github.com/Zeliper/zwan/shared/proto"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newTestServerWith(t, acl.JoinTokens{"secret": acl.DefaultGroup}, nil)
}

// newTestServerWith builds a server with a specific token-to-group table and
// access policy.
func newTestServerWith(t *testing.T, tokens acl.JoinTokens, policy *acl.Policy) *httptest.Server {
	t.Helper()
	alloc, err := ipam.New("100.64.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	nw := store.NewNetwork("demo", "demo.zwan", "100.64.0.0/16")
	ts := httptest.NewServer(New(nw, alloc, tokens, policy, "127.0.0.1:3478").Routes())
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

// ---- access control ----

// aclTokens is a three-group network: dev, guest and nas.
func aclTokens() acl.JoinTokens {
	return acl.JoinTokens{"dev-tok": "dev", "guest-tok": "guest", "nas-tok": "nas"}
}

func peersFor(t *testing.T, base, nodeToken string) map[string]string {
	t.Helper()
	code, body := do(t, http.MethodGet, base+"/v1/peers", nodeToken, nil)
	if code != http.StatusOK {
		t.Fatalf("peers: want 200, got %d", code)
	}
	var pr proto.PeersResponse
	_ = json.Unmarshal(body, &pr)
	out := map[string]string{}
	for _, p := range pr.Peers {
		out[p.Hostname] = p.Group
	}
	return out
}

func serviceNames(t *testing.T, base, nodeToken string) []string {
	t.Helper()
	code, body := do(t, http.MethodGet, base+"/v1/services", nodeToken, nil)
	if code != http.StatusOK {
		t.Fatalf("services: want 200, got %d", code)
	}
	var sr proto.ServicesResponse
	_ = json.Unmarshal(body, &sr)
	var out []string
	for _, s := range sr.Services {
		out = append(out, s.Name)
	}
	return out
}

// The join token decides the group; nothing the client sends does.
func TestGroupComesFromTheJoinToken(t *testing.T) {
	ts := newTestServerWith(t, aclTokens(), nil)

	_, dev := register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})

	groups := peersFor(t, ts.URL, dev.NodeToken)
	if groups["devbox"] != "dev" || groups["guestbox"] != "guest" {
		t.Fatalf("groups = %v", groups)
	}
}

// With no rules the network behaves exactly as it did before ACLs existed.
func TestWithoutRulesEveryoneSeesEveryone(t *testing.T) {
	ts := newTestServerWith(t, aclTokens(), &acl.Policy{})

	_, dev := register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	_, guest := register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})

	if len(peersFor(t, ts.URL, dev.NodeToken)) != 2 || len(peersFor(t, ts.URL, guest.NodeToken)) != 2 {
		t.Fatal("an empty policy must not hide anybody")
	}
}

// Withholding a peer is the enforcement: without the key there is no tunnel.
func TestPeersAreFilteredByPolicy(t *testing.T) {
	policy := &acl.Policy{Rules: []acl.Rule{
		{Src: []string{"dev"}, Dst: []string{"*"}},
		{Src: []string{"guest"}, Dst: []string{"nas"}},
	}}
	ts := newTestServerWith(t, aclTokens(), policy)

	_, dev := register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	_, guest := register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})
	_, nas := register(t, ts.URL, proto.RegisterRequest{Token: "nas-tok", DeviceUUID: "d3", Hostname: "nasbox", PublicKey: "k3"})

	// dev reaches everything.
	if got := peersFor(t, ts.URL, dev.NodeToken); len(got) != 3 {
		t.Fatalf("dev peers = %v, want all three", got)
	}
	// guest may reach nas, and is reachable by dev, but never sees nothing else.
	got := peersFor(t, ts.URL, guest.NodeToken)
	if _, ok := got["guestbox"]; !ok {
		t.Fatal("a member must always see itself")
	}
	if _, ok := got["nasbox"]; !ok {
		t.Fatalf("guest->nas is allowed, so nas must be visible: %v", got)
	}
	if _, ok := got["devbox"]; !ok {
		t.Fatalf("dev->* covers guest, and a tunnel is symmetric, so dev must be visible: %v", got)
	}
	// nas has no outbound rule, but is a destination for both others.
	if len(peersFor(t, ts.URL, nas.NodeToken)) != 3 {
		t.Fatal("nas is reachable from dev and guest, so it sees both")
	}
}

func TestPeersHiddenWhenNoRuleConnectsTwoGroups(t *testing.T) {
	policy := &acl.Policy{Rules: []acl.Rule{{Src: []string{"dev"}, Dst: []string{"nas"}}}}
	ts := newTestServerWith(t, aclTokens(), policy)

	register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	_, guest := register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})

	got := peersFor(t, ts.URL, guest.NodeToken)
	if len(got) != 1 {
		t.Fatalf("guest is in no rule, so it should see only itself: %v", got)
	}
}

func publish(t *testing.T, base, nodeToken string, svc proto.Service) {
	t.Helper()
	code, body := do(t, http.MethodPost, base+"/v1/services", nodeToken, proto.RegisterServiceRequest{Service: svc})
	if code != http.StatusOK {
		t.Fatalf("publish %s: want 200, got %d (%s)", svc.Name, code, body)
	}
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// A service is only listed to members who may reach the node hosting it, so an
// unreachable service never even resolves.
func TestServicesFollowPeerVisibility(t *testing.T) {
	policy := &acl.Policy{Rules: []acl.Rule{{Src: []string{"dev"}, Dst: []string{"nas"}}}}
	ts := newTestServerWith(t, aclTokens(), policy)

	_, dev := register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	_, guest := register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})
	_, nas := register(t, ts.URL, proto.RegisterRequest{Token: "nas-tok", DeviceUUID: "d3", Hostname: "nasbox", PublicKey: "k3"})

	publish(t, ts.URL, nas.NodeToken, proto.Service{Name: "files", Proto: "tcp", Port: 445})

	if !has(serviceNames(t, ts.URL, dev.NodeToken), "files") {
		t.Fatal("dev may reach nas, so it must see the service")
	}
	if has(serviceNames(t, ts.URL, guest.NodeToken), "files") {
		t.Fatal("guest cannot reach nas, so the service must be hidden")
	}
	if !has(serviceNames(t, ts.URL, nas.NodeToken), "files") {
		t.Fatal("a node must always see its own services")
	}
}

// An allow list narrows a service further, independently of the group policy.
func TestServiceAllowListNarrowsAReachableService(t *testing.T) {
	ts := newTestServerWith(t, aclTokens(), nil) // no rules: everyone can reach everyone

	_, dev := register(t, ts.URL, proto.RegisterRequest{Token: "dev-tok", DeviceUUID: "d1", Hostname: "devbox", PublicKey: "k1"})
	_, guest := register(t, ts.URL, proto.RegisterRequest{Token: "guest-tok", DeviceUUID: "d2", Hostname: "guestbox", PublicKey: "k2"})
	_, nas := register(t, ts.URL, proto.RegisterRequest{Token: "nas-tok", DeviceUUID: "d3", Hostname: "nasbox", PublicKey: "k3"})

	publish(t, ts.URL, nas.NodeToken, proto.Service{Name: "open", Proto: "tcp", Port: 80})
	publish(t, ts.URL, nas.NodeToken, proto.Service{Name: "private", Proto: "tcp", Port: 445, AllowGroups: []string{"dev"}})

	devSees, guestSees := serviceNames(t, ts.URL, dev.NodeToken), serviceNames(t, ts.URL, guest.NodeToken)
	if !has(devSees, "open") || !has(devSees, "private") {
		t.Fatalf("dev should see both services: %v", devSees)
	}
	if !has(guestSees, "open") {
		t.Fatalf("guest should still see the unrestricted service: %v", guestSees)
	}
	if has(guestSees, "private") {
		t.Fatalf("guest is outside the allow list: %v", guestSees)
	}
}

func TestServiceAllowListIsStoredAndReturned(t *testing.T) {
	ts := newTestServerWith(t, aclTokens(), nil)
	_, nas := register(t, ts.URL, proto.RegisterRequest{Token: "nas-tok", DeviceUUID: "d3", Hostname: "nasbox", PublicKey: "k3"})

	req := proto.RegisterServiceRequest{Service: proto.Service{Name: "files", Port: 445, AllowGroups: []string{"dev", "  ", ""}}}
	code, body := do(t, http.MethodPost, ts.URL+"/v1/services", nas.NodeToken, req)
	if code != http.StatusOK {
		t.Fatalf("publish: %d (%s)", code, body)
	}
	var out proto.Service
	_ = json.Unmarshal(body, &out)
	if len(out.AllowGroups) != 1 || out.AllowGroups[0] != "dev" {
		t.Fatalf("allow list = %v, want the blanks dropped", out.AllowGroups)
	}
}
