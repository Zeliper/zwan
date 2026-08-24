package l4

import (
	"net/netip"
	"sync"

	"github.com/Zeliper/zwan/shared/acl"
	"github.com/Zeliper/zwan/shared/proto"
)

// AccessPolicy is the live view a running service proxy consults for each
// accepted connection: which group owns each overlay address, and which groups
// each hosted service admits.
//
// It is separate from the proxy so the refresh loop can replace the whole
// picture at once while proxies keep serving — a membership change or an edited
// access list then applies to the next connection instead of needing the proxy
// restarted.
type AccessPolicy struct {
	mu     sync.RWMutex
	groups map[string]string   // overlay IP -> group
	allow  map[string][]string // service name -> allowed groups
}

// Set replaces the policy from the current peer and service lists.
func (a *AccessPolicy) Set(peers []proto.Peer, svcs []proto.Service) {
	groups := make(map[string]string, len(peers))
	for _, p := range peers {
		groups[p.AssignedIP] = p.Group
	}
	allow := make(map[string][]string, len(svcs))
	for _, s := range svcs {
		allow[s.Name] = s.AllowGroups
	}
	a.mu.Lock()
	a.groups, a.allow = groups, allow
	a.mu.Unlock()
}

// Permits reports whether a source address may use a hosted service.
//
// A source with no member record is refused. The peer list a node receives is
// already filtered by the same policy, so an address we were never told about is
// one the control plane decided we should not be talking to.
func (a *AccessPolicy) Permits(service string, src netip.Addr) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	list, ok := a.allow[service]
	if !ok {
		return false // the service is no longer in our view
	}
	if len(list) == 0 {
		return true // open to everyone who can reach this node
	}
	group, known := a.groups[src.String()]
	if !known {
		return false
	}
	return acl.AllowsGroup(list, group)
}

// Filter returns the source check to hand to ListenTCP for one service.
func (a *AccessPolicy) Filter(service string) func(netip.Addr) bool {
	return func(src netip.Addr) bool { return a.Permits(service, src) }
}
