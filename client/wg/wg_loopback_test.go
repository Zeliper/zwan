package wg

import (
	"testing"
	"time"

	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared/keys"
	"golang.zx2c4.com/wireguard/tun/tuntest"
)

// TestHandshakeLoopback reproduces the agent's WireGuard setup with in-memory
// channel TUNs over real loopback UDP (no admin / Wintun needed) and checks that
// the two devices complete a handshake — exactly what the two-client agent test
// on a single host is trying to prove.
func TestHandshakeLoopback(t *testing.T) {
	privA, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}
	privB, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}

	tunA := tuntest.NewChannelTUN()
	tunB := tuntest.NewChannelTUN()
	adA := &tun.Adapter{Name: "A", Dev: tunA.TUN()}
	adB := &tun.Adapter{Name: "B", Dev: tunB.TUN()}

	devA, err := Up(adA, privA, 51820)
	if err != nil {
		t.Fatalf("device A up: %v", err)
	}
	defer devA.Close()
	devB, err := Up(adB, privB, 51821)
	if err != nil {
		t.Fatalf("device B up: %v", err)
	}
	defer devB.Close()

	if err := devA.SetPeers([]Peer{{PublicKeyHex: privB.Public().Hex(), Endpoint: "127.0.0.1:51821", AllowedIP: "100.64.0.2/32"}}); err != nil {
		t.Fatal(err)
	}
	if err := devB.SetPeers([]Peer{{PublicKeyHex: privA.Public().Hex(), Endpoint: "127.0.0.1:51820", AllowedIP: "100.64.0.1/32"}}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		hs, err := devA.PeerHandshakes()
		if err == nil && hs[privB.Public().Hex()] > 0 {
			t.Logf("handshake OK after %v", time.Until(deadline))
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("no handshake within 12s")
}
