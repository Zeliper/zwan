// Package ipc is the control channel between the user-mode tray/GUI and the
// SYSTEM control-server service, over a Windows named pipe (JSON
// request/response).
//
// It is the server-side twin of client/ipc: the GUI configures and supervises
// the hosted network, while the network itself keeps running in the service
// after the window is closed.
package ipc

import (
	"github.com/Zeliper/zwan/server/config"
	"github.com/Zeliper/zwan/shared/proto"
)

// PipeName is the named pipe the server service listens on.
const PipeName = `\\.\pipe\zwan-server`

// State is a snapshot of the hosted network for the UI.
type State struct {
	Running bool          `json:"running"`
	Config  config.Config `json:"config"`
	TLSMode string        `json:"tlsMode"` // "self", "acme" or "off"
	Pin     string        `json:"pin"`     // key fingerprint clients must pin
	JoinURL string        `json:"joinUrl"` // server address + pin, ready to hand out

	// RelayPublic is where clients are told to send tunnel traffic, which is not
	// the same as the relay's listen address and is the one that has to be
	// reachable from outside.
	RelayPublic string `json:"relayPublic"`

	Peers     []proto.Peer    `json:"peers"`
	Services  []proto.Service `json:"services"`
	LastError string          `json:"lastError"`
}

// Request is one IPC call.
type Request struct {
	Op     string         `json:"op"` // "status" | "start" | "stop"
	Config *config.Config `json:"config,omitempty"`
}

// Response is the reply to a Request.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State *State `json:"state,omitempty"`
}

// Handler is implemented by the service to serve IPC requests.
type Handler interface {
	// Start persists the configuration and brings the network up.
	Start(config.Config) error
	Stop() error
	State() State
}
