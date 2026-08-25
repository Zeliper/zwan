package wg

import (
	"strings"
	"testing"
)

func TestBuildPeerConfig(t *testing.T) {
	cfg := buildPeerConfig([]Peer{
		{PublicKeyHex: "aa", Endpoint: "127.0.0.1:51821", AllowedIPs: []string{"100.64.0.2/32"}},
		{PublicKeyHex: "bb", AllowedIPs: []string{"100.64.0.3/32"}}, // no endpoint
	})

	if !strings.HasPrefix(cfg, "replace_peers=true\n") {
		t.Fatalf("missing replace_peers header:\n%s", cfg)
	}
	for _, want := range []string{
		"public_key=aa", "endpoint=127.0.0.1:51821", "allowed_ip=100.64.0.2/32",
		"public_key=bb", "allowed_ip=100.64.0.3/32", "persistent_keepalive_interval=25",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config missing %q:\n%s", want, cfg)
		}
	}
	if strings.Contains(cfg, "endpoint=\n") {
		t.Errorf("empty endpoint should be omitted:\n%s", cfg)
	}
}

// A peer carries the addresses of the services it hosts alongside its own, so
// packets for a service match the tunnel to the node serving it.
func TestBuildPeerConfigCarriesServiceAddresses(t *testing.T) {
	cfg := buildPeerConfig([]Peer{{
		PublicKeyHex: "aa",
		AllowedIPs:   []string{"100.64.0.2/32", "100.64.128.1/32", "100.64.128.2/32"},
	}})

	for _, want := range []string{
		"allowed_ip=100.64.0.2/32", "allowed_ip=100.64.128.1/32", "allowed_ip=100.64.128.2/32",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config missing %q:\n%s", want, cfg)
		}
	}
	if n := strings.Count(cfg, "public_key="); n != 1 {
		t.Errorf("the addresses belong to one peer, got %d peers:\n%s", n, cfg)
	}
}

// A peer with no addresses would match nothing; it should not be written out as
// a peer that silently drops everything.
func TestBuildPeerConfigHandlesAPeerWithNoAddresses(t *testing.T) {
	cfg := buildPeerConfig([]Peer{{PublicKeyHex: "aa"}})
	if strings.Contains(cfg, "allowed_ip=") {
		t.Errorf("no addresses should mean no allowed_ip lines:\n%s", cfg)
	}
}
