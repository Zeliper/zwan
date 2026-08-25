// Package ipc is the control channel between the user-mode tray/GUI and the
// SYSTEM engine service, over a Windows named pipe (JSON request/response).
//
// The unit of work is a network, not a connection: a device can be a member of
// several at once, each identified by the local alias the user gave it.
package ipc

import (
	"github.com/Zeliper/zwan/client/manager"
	"github.com/Zeliper/zwan/client/profile"
)

// PipeName is the named pipe the service listens on.
const PipeName = `\\.\pipe\zwan-engine`

// Request is one IPC call.
type Request struct {
	Op      string           `json:"op"`                // "status" | "connect" | "disconnect" | "forget"
	Network *profile.Network `json:"network,omitempty"` // for "connect"
	Alias   string           `json:"alias,omitempty"`   // for "disconnect" and "forget"
}

// Response is the reply to a Request. Networks is the full list every time, so
// the UI never has to stitch together a picture from separate calls.
type Response struct {
	OK       bool             `json:"ok"`
	Error    string           `json:"error,omitempty"`
	Networks []manager.Status `json:"networks,omitempty"`
}

// Handler is implemented by the service to serve IPC requests. It is exactly
// the manager's surface, so the service adds no logic of its own.
type Handler interface {
	Connect(profile.Network) error
	Disconnect(alias string) error
	Forget(alias string) error
	Statuses() []manager.Status
}
