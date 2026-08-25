package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/manager"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/client/update"
	serverconfig "github.com/Zeliper/zwan/server/config"
	serveripc "github.com/Zeliper/zwan/server/ipc"
	"github.com/Zeliper/zwan/server/supervisor"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/acl"
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

// Networks returns every network this device knows, connected or not.
func (a *App) Networks() ([]manager.Status, error) {
	resp, err := ipc.Status()
	if err != nil {
		return nil, err
	}
	return resp.Networks, nil
}

// Connect joins a network and remembers it. Using an alias that already exists
// replaces that network's settings, so this is also how one is edited.
func (a *App) Connect(n profile.Network) ([]manager.Status, error) {
	return unwrap(ipc.Connect(n))
}

// Disconnect takes one network down but keeps it in the list.
func (a *App) Disconnect(alias string) ([]manager.Status, error) {
	return unwrap(ipc.Disconnect(alias))
}

// Forget removes a network from this device entirely.
func (a *App) Forget(alias string) ([]manager.Status, error) {
	return unwrap(ipc.Forget(alias))
}

// unwrap turns an IPC reply into a result the frontend can use: the network list
// comes back even on failure, so the UI can show what actually happened.
func unwrap(resp ipc.Response, err error) ([]manager.Status, error) {
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return resp.Networks, errors.New(resp.Error)
	}
	return resp.Networks, nil
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

// ParseACL validates the access rules typed in the UI, one
// "<src groups>-><dst groups>" per line, and returns them in the form the server
// stores. Blank lines and #-comments are ignored.
//
// The UI does not parse rules itself: one parser means the text an operator sees
// and the policy the server enforces cannot drift apart.
func (a *App) ParseACL(text string) ([]acl.Rule, error) {
	var rules []acl.Rule
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r, err := acl.ParseRule(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		rules = append(rules, r)
	}
	return rules, nil
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
