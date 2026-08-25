// Package relaytest is an end-to-end integration test: two wireguard-go devices
// (on in-memory channel TUNs) complete a handshake entirely through the server
// relay, using the same relay bind the agent's --relay path uses. No admin or
// Wintun required.
package relaytest

import (
	"net/netip"
	"testing"
	"time"

	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/client/wgbind"
	"github.com/Zeliper/zwan/server/relay"
	"github.com/Zeliper/zwan/shared/keys"
	"golang.zx2c4.com/wireguard/tun/tuntest"
)

func TestRelayHandshake(t *testing.T) {
	r := relay.New()
	ap, err := r.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	defer r.Close()
	go r.Serve()
	relayAddr := ap.String()

	privA, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}
	privB, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}

	bindA, err := wgbind.NewRelay(relayAddr, netip.MustParseAddr("100.64.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	bindB, err := wgbind.NewRelay(relayAddr, netip.MustParseAddr("100.64.0.2"))
	if err != nil {
		t.Fatal(err)
	}

	adA := &tun.Adapter{Name: "A", Dev: tuntest.NewChannelTUN().TUN()}
	adB := &tun.Adapter{Name: "B", Dev: tuntest.NewChannelTUN().TUN()}

	devA, err := wg.Up(adA, privA, 0, bindA)
	if err != nil {
		t.Fatalf("device A up: %v", err)
	}
	defer devA.Close()
	devB, err := wg.Up(adB, privB, 0, bindB)
	if err != nil {
		t.Fatalf("device B up: %v", err)
	}
	defer devB.Close()

	if err := devA.SetPeers([]wg.Peer{{PublicKeyHex: privB.Public().Hex(), Endpoint: "100.64.0.2", AllowedIPs: []string{"100.64.0.2/32"}}}); err != nil {
		t.Fatal(err)
	}
	if err := devB.SetPeers([]wg.Peer{{PublicKeyHex: privA.Public().Hex(), Endpoint: "100.64.0.1", AllowedIPs: []string{"100.64.0.1/32"}}}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		hs, err := devA.PeerHandshakes()
		if err == nil && hs[privB.Public().Hex()] > 0 {
			return // handshake completed through the relay
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("no relay handshake within 12s")
}
