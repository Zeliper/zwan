// Package store holds the control server's in-memory state.
//
// M1a: a single network with its members, kept in memory. Persistence
// (SQLite / modernc.org/sqlite) and multiple networks arrive later.
package store

import (
	"sync"
	"time"
)

// Member is a registered device in a network.
type Member struct {
	DeviceUUID string
	Hostname   string
	PublicKey  string
	AssignedIP string
	Endpoint   string
	JoinedAt   time.Time
}

// Service is a named service reachable at NodeIP:Port over the overlay.
type Service struct {
	Name   string
	Proto  string
	Port   int
	NodeIP string
}

// Network is one overlay network with its members and services.
type Network struct {
	ID          string
	DNSSuffix   string
	OverlayCIDR string

	mu       sync.RWMutex
	members  map[string]*Member  // by DeviceUUID
	services map[string]*Service // by Name
}

// NewNetwork creates an empty network.
func NewNetwork(id, dnsSuffix, overlayCIDR string) *Network {
	return &Network{
		ID:          id,
		DNSSuffix:   dnsSuffix,
		OverlayCIDR: overlayCIDR,
		members:     map[string]*Member{},
		services:    map[string]*Service{},
	}
}

// Upsert inserts or replaces a member by DeviceUUID.
func (n *Network) Upsert(m *Member) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.members[m.DeviceUUID] = m
}

// Members returns a snapshot of the network's members.
func (n *Network) Members() []*Member {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]*Member, 0, len(n.members))
	for _, m := range n.members {
		out = append(out, m)
	}
	return out
}

// UpsertService inserts or replaces a service by Name.
func (n *Network) UpsertService(s *Service) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.services[s.Name] = s
}

// Services returns a snapshot of the network's services.
func (n *Network) Services() []*Service {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]*Service, 0, len(n.services))
	for _, s := range n.services {
		out = append(out, s)
	}
	return out
}
