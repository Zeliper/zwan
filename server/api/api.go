// Package api implements the zwan control-plane HTTP endpoints.
//
// M1a endpoints:
//
//	GET  /healthz      liveness + identity
//	POST /v1/register  join a network (token -> assigned IP)
//	GET  /v1/peers     list members of the network
//
// Auth is a single shared token for now; per-network tokens/passwords and TLS
// (ACME) arrive in later milestones.
package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/store"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
)

// Server wires a single network's store and allocator behind the HTTP API.
type Server struct {
	net       *store.Network
	ipam      *ipam.Allocator
	token     string
	relayAddr string // host:port of the server relay advertised to clients
}

// New builds an API server for one network. relayAddr is the public host:port of
// the server relay (may be empty to disable relay advertisement).
func New(net *store.Network, alloc *ipam.Allocator, token, relayAddr string) *Server {
	return &Server{net: net, ipam: alloc, token: token, relayAddr: relayAddr}
}

// Routes returns the HTTP handler.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/v1/register", s.register)
	mux.HandleFunc("/v1/peers", s.peers)
	mux.HandleFunc("/v1/services", s.services)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"product":   shared.ProductName,
		"component": string(shared.ComponentServer),
		"version":   shared.Version,
	})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req proto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s.token == "" || req.Token != s.token {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if req.DeviceUUID == "" || req.PublicKey == "" {
		writeErr(w, http.StatusBadRequest, "device_uuid and public_key are required")
		return
	}
	ip, err := s.ipam.Allocate(req.DeviceUUID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	s.net.Upsert(&store.Member{
		DeviceUUID: req.DeviceUUID,
		Hostname:   req.Hostname,
		PublicKey:  req.PublicKey,
		AssignedIP: ip.String(),
		Endpoint:   req.Endpoint,
		JoinedAt:   time.Now(),
	})
	writeJSON(w, http.StatusOK, proto.RegisterResponse{
		NetworkID:   s.net.ID,
		DNSSuffix:   s.net.DNSSuffix,
		OverlayCIDR: s.net.OverlayCIDR,
		AssignedIP:  ip.String(),
		RelayAddr:   s.relayAddr,
	})
}

func (s *Server) peers(w http.ResponseWriter, r *http.Request) {
	members := s.net.Members()
	sort.Slice(members, func(i, j int) bool { return members[i].AssignedIP < members[j].AssignedIP })
	resp := proto.PeersResponse{Peers: make([]proto.Peer, 0, len(members))}
	for _, m := range members {
		resp.Peers = append(resp.Peers, proto.Peer{
			Hostname:   m.Hostname,
			PublicKey:  m.PublicKey,
			AssignedIP: m.AssignedIP,
			Endpoint:   m.Endpoint,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// services handles GET (list) and POST (publish) of network services.
func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		svcs := s.net.Services()
		resp := proto.ServicesResponse{Services: make([]proto.Service, 0, len(svcs))}
		for _, sv := range svcs {
			resp.Services = append(resp.Services, proto.Service{
				Name: sv.Name, Proto: sv.Proto, Port: sv.Port, BackendPort: sv.BackendPort, NodeIP: sv.NodeIP,
			})
		}
		sort.Slice(resp.Services, func(i, j int) bool { return resp.Services[i].Name < resp.Services[j].Name })
		writeJSON(w, http.StatusOK, resp)

	case http.MethodPost:
		var req proto.RegisterServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if s.token == "" || req.Token != s.token {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if req.Name == "" || req.NodeIP == "" || req.Port == 0 {
			writeErr(w, http.StatusBadRequest, "name, node_ip and port are required")
			return
		}
		protocol := req.Proto
		if protocol == "" {
			protocol = "tcp"
		}
		s.net.UpsertService(&store.Service{Name: req.Name, Proto: protocol, Port: req.Port, BackendPort: req.BackendPort, NodeIP: req.NodeIP})
		writeJSON(w, http.StatusOK, proto.Service{Name: req.Name, Proto: protocol, Port: req.Port, BackendPort: req.BackendPort, NodeIP: req.NodeIP})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, proto.ErrorResponse{Error: msg})
}
