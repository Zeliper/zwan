// Package proto defines the wire types exchanged between the zwan control
// server and agents over the HTTPS/JSON control channel.
package proto

// RegisterRequest is sent by an agent to join a network.
type RegisterRequest struct {
	Token      string `json:"token"`
	DeviceUUID string `json:"device_uuid"`
	Hostname   string `json:"hostname"`
	PublicKey  string `json:"public_key"`         // base64 X25519 node public key
	Endpoint   string `json:"endpoint,omitempty"` // host:port peers can send WireGuard UDP to (M1b-2)
}

// RegisterResponse is returned on a successful join.
type RegisterResponse struct {
	NetworkID   string `json:"network_id"`
	DNSSuffix   string `json:"dns_suffix"`
	OverlayCIDR string `json:"overlay_cidr"`
	AssignedIP  string `json:"assigned_ip"`
	RelayAddr   string `json:"relay_addr,omitempty"` // host:port of the server relay (M1b-3)

	// NodeToken authenticates this device on every later control call, as
	// "Authorization: Bearer <token>". The join token only authorizes the join
	// itself and is not accepted afterwards.
	NodeToken string `json:"node_token"`
}

// Peer describes another member of the same network.
type Peer struct {
	Hostname   string `json:"hostname"`
	PublicKey  string `json:"public_key"`
	AssignedIP string `json:"assigned_ip"`
	Endpoint   string `json:"endpoint,omitempty"` // host:port for WireGuard (M1b-2)
}

// PeersResponse lists the members of a network.
type PeersResponse struct {
	Peers []Peer `json:"peers"`
}

// Service is a named service in the network, reachable at NodeIP:Port over the
// overlay. Name is the label under the network's DNS suffix (e.g. "minecraft"
// resolves as minecraft.<suffix>).
type Service struct {
	Name        string `json:"name"`
	Proto       string `json:"proto"`                  // "tcp" or "udp"
	Port        int    `json:"port"`                   // port reachable over the overlay (e.g. 25565)
	BackendPort int    `json:"backend_port,omitempty"` // localhost-only backend port on the host (0 = none)
	NodeIP      string `json:"node_ip"`
}

// RegisterServiceRequest publishes (or updates) a service. The caller is
// identified by its node token in the Authorization header.
type RegisterServiceRequest struct {
	Service
}

// ServicesResponse lists a network's services.
type ServicesResponse struct {
	Services []Service `json:"services"`
}

// ErrorResponse is returned for non-2xx control-plane responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
