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
	JoinedAt   time.Time
}

// Network is one overlay network and its members (keyed by DeviceUUID).
type Network struct {
	ID          string
	DNSSuffix   string
	OverlayCIDR string

	mu      sync.RWMutex
	members map[string]*Member
}

// NewNetwork creates an empty network.
func NewNetwork(id, dnsSuffix, overlayCIDR string) *Network {
	return &Network{
		ID:          id,
		DNSSuffix:   dnsSuffix,
		OverlayCIDR: overlayCIDR,
		members:     map[string]*Member{},
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
