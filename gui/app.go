package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/update"
	serverconfig "github.com/Zeliper/zwan/server/config"
	serveripc "github.com/Zeliper/zwan/server/ipc"
	"github.com/Zeliper/zwan/server/supervisor"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails backend. It is both a client control surface (over the engine
// service via client/ipc) and an in-process server host (server/host), so one
// app can join networks and host one.
type App struct {
	ctx context.Context

	mu  sync.Mutex
	srv *supervisor.Supervisor // in-process fallback; nil until the service is missing
}

// NewApp creates the App.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// ---- client (engine service via IPC) ----

// ServiceUp reports whether the engine service is reachable.
func (a *App) ServiceUp() bool {
	_, err := ipc.Status()
	return err == nil
}

// Connect asks the service to join and bring up a network. pin is the control
// server's key fingerprint, needed when it has no CA-issued certificate; it may
// be left empty and pasted inside server as a "#<pin>" fragment instead.
func (a *App) Connect(server, pin, token, name string, useRelay bool) (*engine.Status, error) {
	resp, err := ipc.Connect(ipc.ConnectArgs{Server: server, Pin: pin, Token: token, Name: name, UseRelay: useRelay})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return resp.Status, errors.New(resp.Error)
	}
	return resp.Status, nil
}

// Disconnect tears the current connection down.
func (a *App) Disconnect() (*engine.Status, error) {
	resp, err := ipc.Disconnect()
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}

// Status returns the current connection status from the service.
func (a *App) Status() (*engine.Status, error) {
	resp, err := ipc.Status()
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}

// ---- server (control-server service, or in-process as a fallback) ----

// HostState is the server view for the frontend.
type HostState struct {
	Running   bool                `json:"running"`
	Config    serverconfig.Config `json:"config"`
	TLSMode   string              `json:"tlsMode"` // "self", "acme" or "off"
	Pin       string              `json:"pin"`     // key fingerprint clients must pin
	JoinURL   string              `json:"joinUrl"` // server address + pin, ready to hand out
	Peers     []proto.Peer        `json:"peers"`
	Services  []proto.Service     `json:"services"`
	LastError string              `json:"lastError"`

	// ManagedByService is false when this app is hosting the network itself
	// because the server service is not installed — the network then stops when
	// the app quits, and the UI says so.
	ManagedByService bool `json:"managedByService"`
}

// ServerServiceUp reports whether the control-server service is reachable.
func (a *App) ServerServiceUp() bool {
	_, err := serveripc.Status()
	return err == nil
}

// HostGenToken returns a fresh random join token.
func (a *App) HostGenToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HostStart starts hosting a network, preferring the service so the network
// outlives this window.
func (a *App) HostStart(cfg serverconfig.Config) error {
	if a.ServerServiceUp() {
		resp, err := serveripc.Start(cfg)
		if err != nil {
			return err
		}
		if !resp.OK {
			return errors.New(resp.Error)
		}
		return nil
	}
	return a.local().Start(cfg)
}

// HostStop stops hosting.
func (a *App) HostStop() error {
	if a.ServerServiceUp() {
		resp, err := serveripc.Stop()
		if err != nil {
			return err
		}
		if !resp.OK {
			return errors.New(resp.Error)
		}
		return nil
	}
	return a.local().Stop()
}

// HostStatus returns the hosting state plus current members and services.
func (a *App) HostStatus() *HostState {
	if a.ServerServiceUp() {
		if resp, err := serveripc.Status(); err == nil && resp.State != nil {
			return hostState(*resp.State, true)
		}
	}
	return hostState(a.local().State(), false)
}

// local lazily creates the in-process supervisor used when the service is absent.
func (a *App) local() *supervisor.Supervisor {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.srv == nil {
		a.srv = supervisor.New()
	}
	return a.srv
}

func hostState(st serveripc.State, managed bool) *HostState {
	return &HostState{
		Running: st.Running, Config: st.Config, TLSMode: st.TLSMode, Pin: st.Pin,
		JoinURL: st.JoinURL, Peers: st.Peers, Services: st.Services, LastError: st.LastError,
		ManagedByService: managed,
	}
}

// ---- version / auto-update ----

// Version returns the app version.
func (a *App) Version() string { return shared.Version }

// CheckUpdate returns a newer release if one is available, else nil.
func (a *App) CheckUpdate() *update.Release {
	rel, err := update.Latest()
	if err != nil || rel == nil {
		return nil
	}
	if !update.IsNewer(shared.Version, rel.Version) {
		return nil
	}
	return rel
}

// ApplyUpdate downloads and launches the latest installer (silent, elevated).
func (a *App) ApplyUpdate(installerURL string) error { return update.Apply(installerURL) }

// QuitApp exits the application (used after launching an update).
func (a *App) QuitApp() {
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}
