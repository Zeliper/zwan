package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/shared/proto"
)

// App is the Wails backend. It is both a client control surface (over the engine
// service via client/ipc) and an in-process server host (server/host), so one
// app can join networks and host one.
type App struct {
	ctx context.Context
	srv *host.Host
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

// Connect asks the service to join and bring up a network.
func (a *App) Connect(server, token, name string, useRelay bool) (*engine.Status, error) {
	resp, err := ipc.Connect(ipc.ConnectArgs{Server: server, Token: token, Name: name, UseRelay: useRelay})
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
	Peers       []proto.Peer    `json:"peers"`
	Services    []proto.Service `json:"services"`
}

// HostGenToken returns a fresh random join token.
func (a *App) HostGenToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HostStart starts hosting a network in-process.
func (a *App) HostStart(networkID, suffix, cidr, token, controlAddr, relayAddr string) error {
	return a.srv.Start(host.Config{
		NetworkID:   networkID,
		DNSSuffix:   suffix,
		CIDR:        cidr,
		Token:       token,
		ControlAddr: controlAddr,
		RelayAddr:   relayAddr,
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
	}
	if a.srv.Running() {
		base := "http://" + localAddr(c.ControlAddr)
		if p, err := join.FetchPeers(base); err == nil {
			st.Peers = p
		}
		if s, err := join.FetchServices(base); err == nil {
			st.Services = s
		}
	}
	return st
}

// localAddr rewrites a wildcard listen address to loopback for local queries.
func localAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
