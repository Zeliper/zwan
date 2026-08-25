package manager

import (
	"testing"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/shared"
)

// isolated points the manager's storage at a temporary directory so tests never
// touch the machine's real network list.
func isolated(t *testing.T) *Manager {
	t.Helper()
	t.Setenv(shared.StateDirEnv, t.TempDir())
	return New(Config{ProductName: "zwan", DeviceUUID: "test-device"})
}

// A network is remembered even when it cannot be brought up. Losing the entry
// because a server was unreachable would make the device forget where it
// belongs; the next Start retries instead.
func TestConnectRemembersANetworkThatFailedToStart(t *testing.T) {
	m := isolated(t)

	err := m.Connect(profile.Network{Alias: "Alice", Server: "https://127.0.0.1:1", Token: "t"})
	if err == nil {
		t.Fatal("connecting to a dead server should fail")
	}

	saved, err := profile.LoadNetworks()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].Alias != "alice" {
		t.Fatalf("saved = %+v, want the normalized alias remembered", saved)
	}
	if !saved[0].AutoConnect {
		t.Fatal("connecting marks a network as wanted, so it comes back on restart")
	}

	st := m.Statuses()
	if len(st) != 1 || st[0].Engine.Connected {
		t.Fatalf("statuses = %+v, want one known but disconnected network", st)
	}
}

func TestConnectRejectsAnUnusableAlias(t *testing.T) {
	m := isolated(t)
	if err := m.Connect(profile.Network{Alias: "not a name", Server: "https://x", Token: "t"}); err == nil {
		t.Fatal("an alias that cannot be a DNS suffix must be refused")
	}
	if saved, _ := profile.LoadNetworks(); len(saved) != 0 {
		t.Fatalf("a rejected network must not be stored: %+v", saved)
	}
}

func TestDisconnectKeepsTheNetworkButClearsAutoConnect(t *testing.T) {
	m := isolated(t)
	_ = m.Connect(profile.Network{Alias: "alice", Server: "https://127.0.0.1:1", Token: "t"})

	if err := m.Disconnect("Alice"); err != nil {
		t.Fatal(err)
	}
	saved, _ := profile.LoadNetworks()
	if len(saved) != 1 {
		t.Fatalf("disconnect should keep the network: %+v", saved)
	}
	if saved[0].AutoConnect {
		t.Fatal("disconnect means it should stay down across a restart")
	}
}

func TestForgetRemovesTheNetwork(t *testing.T) {
	m := isolated(t)
	_ = m.Connect(profile.Network{Alias: "alice", Server: "https://127.0.0.1:1", Token: "t"})
	_ = m.Connect(profile.Network{Alias: "bob", Server: "https://127.0.0.1:1", Token: "t"})

	if err := m.Forget("alice"); err != nil {
		t.Fatal(err)
	}
	saved, _ := profile.LoadNetworks()
	if len(saved) != 1 || saved[0].Alias != "bob" {
		t.Fatalf("saved = %+v, want only bob", saved)
	}
}

// Reconnecting an alias replaces its settings rather than adding a duplicate.
func TestConnectingTheSameAliasReplacesIt(t *testing.T) {
	m := isolated(t)
	_ = m.Connect(profile.Network{Alias: "alice", Server: "https://127.0.0.1:1", Token: "old"})
	_ = m.Connect(profile.Network{Alias: "alice", Server: "https://127.0.0.1:1", Token: "new"})

	saved, _ := profile.LoadNetworks()
	if len(saved) != 1 || saved[0].Token != "new" {
		t.Fatalf("saved = %+v, want one entry with the new token", saved)
	}
}

// Each network needs its own UDP port; sharing one would stop the second tunnel.
func TestFreePortsAreDistinct(t *testing.T) {
	m := isolated(t)
	m.mu.Lock()
	defer m.mu.Unlock()

	first := m.freePortLocked()
	if first == 0 {
		t.Skip("no free port in the probe range on this machine")
	}
	m.nets["a"] = &entry{port: first}
	second := m.freePortLocked()
	if second == first {
		t.Fatalf("both networks got port %d", first)
	}
}

// Two networks on the same range can hand out the same address, and the host has
// one routing table. Report it rather than let a peer quietly go missing.
func TestOverlappingRangesAreReported(t *testing.T) {
	list := []Status{
		{Network: profile.Network{Alias: "alice"}, Engine: engine.Status{Connected: true, OverlayCIDR: "100.64.0.0/16"}},
		{Network: profile.Network{Alias: "bob"}, Engine: engine.Status{Connected: true, OverlayCIDR: "100.64.0.0/16"}},
		{Network: profile.Network{Alias: "carol"}, Engine: engine.Status{Connected: true, OverlayCIDR: "100.77.0.0/16"}},
	}
	addOverlapWarnings(list)

	if list[0].Warning == "" || list[1].Warning == "" {
		t.Fatalf("overlapping networks should both be flagged: %+v", list)
	}
	if list[2].Warning != "" {
		t.Fatalf("a distinct range must not be flagged: %q", list[2].Warning)
	}
}

func TestDisconnectedNetworksAreNotComparedForOverlap(t *testing.T) {
	list := []Status{
		{Network: profile.Network{Alias: "alice"}, Engine: engine.Status{Connected: true, OverlayCIDR: "100.64.0.0/16"}},
		{Network: profile.Network{Alias: "bob"}, Engine: engine.Status{OverlayCIDR: "100.64.0.0/16"}},
	}
	addOverlapWarnings(list)
	if list[0].Warning != "" || list[1].Warning != "" {
		t.Fatalf("a network that is down cannot collide with anything: %+v", list)
	}
}
