package engine

import (
	"fmt"
	"net/netip"

	"github.com/Zeliper/zwan/client/vip"
	"github.com/Zeliper/zwan/shared/proto"
)

// translator converts between the addresses a network really uses and the
// addresses this device uses for it.
//
// The zero value is the identity, which is what a device running a single
// network wants: no translation, no surprises in the routing table. It becomes a
// real mapping only when the caller asks for a local range, because that is the
// only situation where two networks could hand out the same address.
type translator struct{ table *vip.Table }

// newTranslator builds a translator for a local range, or the identity when no
// range is configured.
func newTranslator(localPrefix, overlayCIDR string) (translator, error) {
	if localPrefix == "" {
		return translator{}, nil
	}
	local, err := netip.ParsePrefix(localPrefix)
	if err != nil {
		return translator{}, fmt.Errorf("local range %q: %w", localPrefix, err)
	}
	overlay, err := netip.ParsePrefix(overlayCIDR)
	if err != nil {
		return translator{}, fmt.Errorf("overlay range %q: %w", overlayCIDR, err)
	}
	table, err := vip.NewTable(local, overlay)
	if err != nil {
		return translator{}, err
	}
	return translator{table: table}, nil
}

func (t translator) on() bool { return t.table != nil }

// cidr is the local range in use, or "" when translating is off.
func (t translator) cidr() string {
	if !t.on() {
		return ""
	}
	return t.table.Prefix().String()
}

// local converts one overlay address. It reports false for an address this
// network has no room for, which the caller treats as "skip this peer" rather
// than routing it to something else.
func (t translator) local(overlay string) (string, bool) {
	if !t.on() {
		return overlay, overlay != ""
	}
	addr, err := netip.ParseAddr(overlay)
	if err != nil {
		return "", false
	}
	got, ok := t.table.Local(addr)
	if !ok {
		return "", false
	}
	return got.String(), true
}

// peers restates a peer list in the addresses this device uses, so everything
// the operating system and the user see agrees.
func (t translator) peers(in []proto.Peer) []proto.Peer {
	if !t.on() {
		return in
	}
	out := make([]proto.Peer, 0, len(in))
	for _, p := range in {
		local, ok := t.local(p.AssignedIP)
		if !ok {
			continue
		}
		p.AssignedIP = local
		out = append(out, p)
	}
	return out
}

// services does the same for the service registry, so a name resolves to an
// address the host can actually route.
func (t translator) services(in []proto.Service) []proto.Service {
	if !t.on() {
		return in
	}
	out := make([]proto.Service, 0, len(in))
	for _, s := range in {
		local, ok := t.local(s.NodeIP)
		if !ok {
			continue
		}
		s.NodeIP = local
		if s.VIP != "" {
			// A service's own address is an overlay address like any other, so
			// it is translated the same way; without a mapping the service is
			// simply not offered.
			vip, ok := t.local(s.VIP)
			if !ok {
				continue
			}
			s.VIP = vip
		}
		out = append(out, s)
	}
	return out
}

// handshakes re-keys tunnel state by the addresses the rest of the UI uses.
func (t translator) handshakes(in map[string]bool) map[string]bool {
	if !t.on() {
		return in
	}
	out := make(map[string]bool, len(in))
	for overlay, up := range in {
		if local, ok := t.local(overlay); ok {
			out[local] = up
		}
	}
	return out
}
