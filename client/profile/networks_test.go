package profile

import (
	"reflect"
	"testing"

	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
)

func TestNetworksRoundTrip(t *testing.T) {
	t.Setenv(shared.StateDirEnv, t.TempDir())

	if got, err := LoadNetworks(); err != nil || got != nil {
		t.Fatalf("a device with no saved networks = (%v, %v)", got, err)
	}

	want := []Network{
		{Alias: "alice", Server: "https://a.test:8787", Pin: "sha256:x", Token: "t1", AutoConnect: true},
		{Alias: "bob", Server: "https://b.test:8787", Token: "t2", UseRelay: true},
	}
	if err := SaveNetworks(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadNetworks()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestValidAlias(t *testing.T) {
	for _, ok := range []string{"alice", "home-lab", "a", "office.corp", "n7"} {
		if err := ValidAlias(ok); err != nil {
			t.Fatalf("ValidAlias(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "  ", "-lead", "trail-", "has space", "under_score", "a..b", "UPPER!"} {
		if err := ValidAlias(bad); err == nil {
			t.Fatalf("ValidAlias(%q) accepted an unusable DNS suffix", bad)
		}
	}
	// Case and stray dots are normalized rather than rejected.
	if err := ValidAlias("  Alice.  "); err != nil {
		t.Fatalf("ValidAlias should normalize before checking: %v", err)
	}
	if got := NormalizeAlias("  Alice.  "); got != "alice" {
		t.Fatalf("NormalizeAlias = %q", got)
	}
}

func TestNetworkValidate(t *testing.T) {
	base := Network{Alias: "alice", Server: "https://a.test", Token: "t"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	noServer := base
	noServer.Server = ""
	if err := noServer.Validate(); err == nil {
		t.Fatal("a network without a server should be rejected")
	}
	noToken := base
	noToken.Token = ""
	if err := noToken.Validate(); err == nil {
		t.Fatal("a network without a token should be rejected")
	}
}

func TestValidServiceAcceptsWhatCanBePublished(t *testing.T) {
	ok := []proto.Service{
		{Name: "minecraft", Proto: "tcp", Port: 25565, BackendPort: 31001},
		{Name: "voice", Proto: "udp", Port: 64738, BackendPort: 31003},
		{Name: "nas-01", Port: 445},                           // protocol defaults to tcp
		{Name: "web", Proto: "TCP", Port: 80, BackendPort: 0}, // no proxy: it binds the address itself
	}
	for _, s := range ok {
		if err := ValidService(s); err != nil {
			t.Errorf("ValidService(%+v) = %v, want nil", s, err)
		}
	}
}

// The name becomes a DNS label under the network's local name, so anything that
// cannot be one has to be refused where the user can still see why.
func TestValidServiceRejectsWhatCannotBeAName(t *testing.T) {
	bad := []proto.Service{
		{Name: "", Port: 80},
		{Name: "my service", Port: 80},
		{Name: "-lead", Port: 80},
		{Name: "trail-", Port: 80},
		{Name: "게임서버", Port: 80},
		{Name: "ok", Port: 0},
		{Name: "ok", Port: 70000},
		{Name: "ok", Port: 80, BackendPort: 70000},
		{Name: "ok", Proto: "sctp", Port: 80},
	}
	for _, s := range bad {
		if err := ValidService(s); err == nil {
			t.Errorf("ValidService(%+v) was accepted; it should not be", s)
		}
	}
}

// Two services with the same name would resolve to one address and quietly
// shadow each other, so the collision is caught before it is stored.
func TestValidateRejectsDuplicateServiceNames(t *testing.T) {
	n := Network{Alias: "home", Server: "https://h:8787", Token: "t", Publish: []proto.Service{
		{Name: "echo", Port: 1000, Proto: "tcp"},
		{Name: "Echo", Port: 2000, Proto: "tcp"},
	}}
	if err := n.Validate(); err == nil {
		t.Fatal("two services named the same were accepted")
	}
}

func TestNormalizePublishFillsInTheDefaults(t *testing.T) {
	got := NormalizePublish([]proto.Service{
		{Name: "  Echo ", Port: 25565, NodeIP: "100.64.0.9", VIP: "100.64.128.9",
			AllowGroups: []string{" dev ", "", "guest"}},
	})
	if len(got) != 1 {
		t.Fatalf("got %d services, want 1", len(got))
	}
	s := got[0]
	if s.Name != "echo" {
		t.Errorf("name = %q, want %q", s.Name, "echo")
	}
	if s.Proto != "tcp" {
		t.Errorf("proto = %q, want tcp by default", s.Proto)
	}
	// The addresses are the server's to decide; anything a caller sent is stale
	// the moment one is reassigned.
	if s.NodeIP != "" || s.VIP != "" {
		t.Errorf("addresses survived normalisation: nodeIP=%q vip=%q", s.NodeIP, s.VIP)
	}
	if len(s.AllowGroups) != 2 || s.AllowGroups[0] != "dev" || s.AllowGroups[1] != "guest" {
		t.Errorf("allow groups = %v, want [dev guest]", s.AllowGroups)
	}
}

// Published services have to survive the file, because that file is what puts
// them back. The server's registry lives in memory, so after either end restarts
// this list is the only record that the device offered anything at all.
func TestPublishedServicesSurviveTheFile(t *testing.T) {
	t.Setenv(shared.StateDirEnv, t.TempDir())

	want := []Network{{
		Alias: "home", Server: "https://h:8787", Token: "t", AutoConnect: true,
		Publish: []proto.Service{
			{Name: "echo", Proto: "tcp", Port: 25565, BackendPort: 31001},
			{Name: "voice", Proto: "udp", Port: 64738, BackendPort: 31003, AllowGroups: []string{"dev"}},
		},
	}}
	if err := SaveNetworks(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadNetworks()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the networks:\n got %+v\nwant %+v", got, want)
	}
}
