package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/shared/proto"
)

// App is the Wails backend for the zwan desktop client.
//
// This first version drives the control plane (join a network, list peers and
// services). Bringing up the encrypted tunnel requires Administrator and is
// wired in a follow-up (engine-as-service + IPC).
type App struct {
	ctx context.Context

	mu     sync.Mutex
	server string
	token  string
	last   *JoinResult
}

// PeerInfo is a network member, for the frontend.
type PeerInfo struct {
	Hostname   string `json:"hostname"`
	PublicKey  string `json:"publicKey"`
	AssignedIP string `json:"assignedIp"`
	Endpoint   string `json:"endpoint"`
}

// ServiceInfo is a published service, for the frontend.
type ServiceInfo struct {
	Name        string `json:"name"`
	Proto       string `json:"proto"`
	Port        int    `json:"port"`
	BackendPort int    `json:"backendPort"`
	NodeIP      string `json:"nodeIp"`
	FQDN        string `json:"fqdn"`
}

// JoinResult is the state shown after joining a network.
type JoinResult struct {
	NetworkID   string        `json:"networkId"`
	DNSSuffix   string        `json:"dnsSuffix"`
	OverlayCIDR string        `json:"overlayCidr"`
	AssignedIP  string        `json:"assignedIp"`
	RelayAddr   string        `json:"relayAddr"`
	PublicKey   string        `json:"publicKey"`
	DeviceUUID  string        `json:"deviceUuid"`
	Peers       []PeerInfo    `json:"peers"`
	Services    []ServiceInfo `json:"services"`
}

// NewApp creates the App.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// DeviceID returns this machine's stable device identifier.
func (a *App) DeviceID() (string, error) { return profile.DeviceUUID() }

// Join registers this device with the control server and returns the network
// state. server is the base URL (e.g. http://host:8787); name is the hostname
// label shown to peers.
func (a *App) Join(server, token, name string) (*JoinResult, error) {
	if server == "" || token == "" {
		return nil, fmt.Errorf("server and token are required")
	}
	dev, err := profile.DeviceUUID()
	if err != nil {
		return nil, fmt.Errorf("device id: %w", err)
	}
	res, err := join.Do(server, token, dev, name, "")
	if err != nil {
		return nil, err
	}

	jr := &JoinResult{
		NetworkID:   res.Register.NetworkID,
		DNSSuffix:   res.Register.DNSSuffix,
		OverlayCIDR: res.Register.OverlayCIDR,
		AssignedIP:  res.Register.AssignedIP,
		RelayAddr:   res.Register.RelayAddr,
		PublicKey:   res.PublicKey,
		DeviceUUID:  dev,
	}
	jr.Peers = toPeers(res.Peers)
	if svcs, err := join.FetchServices(server); err == nil {
		jr.Services = toServices(svcs, res.Register.DNSSuffix)
	}

	a.mu.Lock()
	a.server, a.token, a.last = server, token, jr
	a.mu.Unlock()
	return jr, nil
}

// Refresh re-fetches peers and services for the joined network.
func (a *App) Refresh() (*JoinResult, error) {
	a.mu.Lock()
	server, last := a.server, a.last
	a.mu.Unlock()
	if server == "" || last == nil {
		return nil, fmt.Errorf("not joined yet")
	}
	peers, err := join.FetchPeers(server)
	if err != nil {
		return nil, err
	}
	last.Peers = toPeers(peers)
	if svcs, err := join.FetchServices(server); err == nil {
		last.Services = toServices(svcs, last.DNSSuffix)
	}
	return last, nil
}

func toPeers(in []proto.Peer) []PeerInfo {
	out := make([]PeerInfo, 0, len(in))
	for _, p := range in {
		out = append(out, PeerInfo{Hostname: p.Hostname, PublicKey: p.PublicKey, AssignedIP: p.AssignedIP, Endpoint: p.Endpoint})
	}
	return out
}

func toServices(in []proto.Service, suffix string) []ServiceInfo {
	out := make([]ServiceInfo, 0, len(in))
	for _, s := range in {
		out = append(out, ServiceInfo{
			Name: s.Name, Proto: s.Proto, Port: s.Port, BackendPort: s.BackendPort, NodeIP: s.NodeIP,
			FQDN: s.Name + "." + suffix,
		})
	}
	return out
}
