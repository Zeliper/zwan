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

// ErrorResponse is returned for non-2xx control-plane responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
