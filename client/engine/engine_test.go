package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/shared/proto"
)

// registryStub records what gets published, standing in for the control server.
type registryStub struct {
	mu        sync.Mutex
	published []proto.Service
	fail      bool
}

func (r *registryStub) client(t *testing.T) *join.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/services", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.fail {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(proto.ErrorResponse{Error: "nope"})
			return
		}
		var in proto.RegisterServiceRequest
		_ = json.NewDecoder(req.Body).Decode(&in)
		r.published = append(r.published, in.Service)
		_ = json.NewEncoder(w).Encode(in.Service)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cl, err := join.NewClient(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

func (r *registryStub) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, s := range r.published {
		out = append(out, s.Name)
	}
	return out
}

// The registry lives in the control server's memory, so a restart drops every
// service. The engine has to put back what this node hosts.
func TestRepublishRestoresServicesTheServerLost(t *testing.T) {
	stub := &registryStub{}
	cl := stub.client(t)

	want := []proto.Service{
		{Name: "files", Proto: "tcp", Port: 445, BackendPort: 31445},
		{Name: "game", Proto: "tcp", Port: 25565},
	}
	got := republish(cl, want, nil, "100.64.0.7")

	if names := stub.names(); len(names) != 2 {
		t.Fatalf("published %v, want both services", names)
	}
	if len(got) != 2 {
		t.Fatalf("returned list = %+v, want both services", got)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for _, s := range stub.published {
		if s.NodeIP != "100.64.0.7" {
			t.Fatalf("published %s with node IP %q, want this node's address", s.Name, s.NodeIP)
		}
	}
}

// A service the server already lists must not be published again on every tick.
func TestRepublishLeavesRegisteredServicesAlone(t *testing.T) {
	stub := &registryStub{}
	cl := stub.client(t)

	want := []proto.Service{{Name: "files", Proto: "tcp", Port: 445}}
	have := []proto.Service{{Name: "files", Proto: "tcp", Port: 445, NodeIP: "100.64.0.7"}}

	got := republish(cl, want, have, "100.64.0.7")
	if names := stub.names(); len(names) != 0 {
		t.Fatalf("published %v, want nothing", names)
	}
	if len(got) != 1 {
		t.Fatalf("returned list = %+v, want the existing entry", got)
	}
}

func TestRepublishWithNothingToPublishIsANoOp(t *testing.T) {
	stub := &registryStub{}
	cl := stub.client(t)

	have := []proto.Service{{Name: "other", NodeIP: "100.64.0.9"}}
	got := republish(cl, nil, have, "100.64.0.7")
	if len(got) != 1 || got[0].Name != "other" {
		t.Fatalf("returned list = %+v, want the server's list untouched", got)
	}
	if names := stub.names(); len(names) != 0 {
		t.Fatalf("published %v, want nothing", names)
	}
}

// A refused publish must not be reported as registered, or the engine would
// stop retrying something that never landed.
func TestRepublishSkipsServicesTheServerRefuses(t *testing.T) {
	stub := &registryStub{fail: true}
	cl := stub.client(t)

	want := []proto.Service{{Name: "files", Proto: "tcp", Port: 445}}
	if got := republish(cl, want, nil, "100.64.0.7"); len(got) != 0 {
		t.Fatalf("returned list = %+v, want the refused service left out", got)
	}
}
