package wg

import (
	"strings"
	"testing"
)

func TestBuildPeerConfig(t *testing.T) {
	cfg := buildPeerConfig([]Peer{
		{PublicKeyHex: "aa", Endpoint: "127.0.0.1:51821", AllowedIP: "100.64.0.2/32"},
		{PublicKeyHex: "bb", AllowedIP: "100.64.0.3/32"}, // no endpoint
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
