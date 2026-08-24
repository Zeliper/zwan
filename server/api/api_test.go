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
	return httptest.NewServer(New(nw, alloc, "secret", "127.0.0.1:3478").Routes())
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

func TestRegisterInvalidToken(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	code, _ := register(t, ts.URL, proto.RegisterRequest{Token: "wrong", DeviceUUID: "d1", PublicKey: "k1"})
	if code != http.StatusUnauthorized {
		t.Fatalf("want 401 for bad token, got %d", code)
	}
}

func TestRegisterRequiresFields(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	code, _ := register(t, ts.URL, proto.RegisterRequest{Token: "secret", DeviceUUID: "", PublicKey: ""})
	if code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing fields, got %d", code)
	}
}

func TestRegisterAllocatesStableIPAndListsPeers(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

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

	resp, err := http.Get(ts.URL + "/v1/peers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var pr proto.PeersResponse
	_ = json.NewDecoder(resp.Body).Decode(&pr)
	if len(pr.Peers) != 2 {
		t.Fatalf("want 2 peers, got %d", len(pr.Peers))
	}
}
