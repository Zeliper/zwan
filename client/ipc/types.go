// Package ipc is the control channel between the user-mode tray/GUI and the
// SYSTEM engine service, over a Windows named pipe (JSON request/response).
package ipc

import "github.com/Zeliper/zwan/client/engine"

// PipeName is the named pipe the service listens on.
const PipeName = `\\.\pipe\zwan-engine`

// ConnectArgs are the fields the UI supplies to join/connect a network.
type ConnectArgs struct {
	Server   string `json:"server"`
	Token    string `json:"token"`
	Name     string `json:"name"`
	UseRelay bool   `json:"useRelay"`
}

// Request is one IPC call.
type Request struct {
	Op      string       `json:"op"` // "connect" | "disconnect" | "status"
	Connect *ConnectArgs `json:"connect,omitempty"`
}

// Response is the reply to a Request.
type Response struct {
	OK     bool           `json:"ok"`
	Error  string         `json:"error,omitempty"`
	Status *engine.Status `json:"status,omitempty"`
}

// Handler is implemented by the service to serve IPC requests.
type Handler interface {
	Connect(ConnectArgs) error
	Disconnect() error
	Status() engine.Status
}
