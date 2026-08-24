package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/update"
	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails backend. It is both a client control surface (over the engine
// service via client/ipc) and an in-process server host (server/host), so one
// app can join networks and host one.
type App struct {
	ctx        context.Context
	srv        *host.Host
	publicHost string // host:port to advertise in the join address
}

// NewApp creates the App.
func NewApp() *App { return &App{srv: host.New()} }

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

// ---- server (in-process host) ----

// HostState is the server view for the frontend.
type HostState struct {
	Running     bool            `json:"running"`
	NetworkID   string          `json:"networkId"`
	DNSSuffix   string          `json:"dnsSuffix"`
	CIDR        string          `json:"cidr"`
	Token       string          `json:"token"`
	ControlAddr string          `json:"controlAddr"`
	RelayAddr   string          `json:"relayAddr"`
	TLSMode     string          `json:"tlsMode"` // "self", "acme" or "off"
	Pin         string          `json:"pin"`     // key fingerprint clients must pin
	JoinURL     string          `json:"joinUrl"` // server address + pin, ready to hand out
	Peers       []proto.Peer    `json:"peers"`
	Services    []proto.Service `json:"services"`
}

// HostGenToken returns a fresh random join token.
func (a *App) HostGenToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HostStart starts hosting a network in-process. tlsMode is "auto", "self",
// "acme" or "off"; domain is a comma-separated list of public hostnames for an
// ACME certificate, and publicHost overrides the address shown to joiners.
func (a *App) HostStart(networkID, suffix, cidr, token, controlAddr, relayAddr, tlsMode, domain, publicHost string) error {
	a.publicHost = strings.TrimSpace(publicHost)
	return a.srv.Start(host.Config{
		NetworkID:   networkID,
		DNSSuffix:   suffix,
		CIDR:        cidr,
		Token:       token,
		ControlAddr: controlAddr,
		RelayAddr:   relayAddr,
		TLSMode:     tlsMode,
		TLSDomains:  splitList(domain),
	})
}

// HostStop stops hosting.
func (a *App) HostStop() { a.srv.Stop() }

// HostStatus returns the hosting state plus current members and services.
func (a *App) HostStatus() *HostState {
	c := a.srv.Config()
	st := &HostState{
		Running: a.srv.Running(), NetworkID: c.NetworkID, DNSSuffix: c.DNSSuffix, CIDR: c.CIDR,
		Token: c.Token, ControlAddr: c.ControlAddr, RelayAddr: c.RelayAddr,
		TLSMode: a.srv.TLSMode(), Pin: a.srv.Pin(), JoinURL: a.srv.JoinURL(a.publicHost),
	}
	if a.srv.Running() {
		// Query our own control API the same way a client would, pin included.
		if cl, err := join.NewClient(a.srv.LocalURL(), a.srv.Pin()); err == nil {
			if p, err := cl.Peers(); err == nil {
				st.Peers = p
			}
			if s, err := cl.Services(); err == nil {
				st.Services = s
			}
		}
	}
	return st
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

// splitList parses a comma-separated field from the UI.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
