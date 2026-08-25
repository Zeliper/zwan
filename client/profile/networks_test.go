package profile

import (
	"reflect"
	"testing"

	"github.com/Zeliper/zwan/shared"
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
