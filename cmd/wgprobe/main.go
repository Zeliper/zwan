// Command wgprobe is a diagnostic: one wireguard-go device on an in-memory
// channel TUN (no admin / Wintun), talking to another wgprobe over loopback UDP.
// Run two processes (--id a and --id b) to reproduce the two-process handshake
// path without needing Administrator. Set ZWAN_WG_LOG=verbose for WG internals.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/shared/keys"
	"golang.zx2c4.com/wireguard/tun/tuntest"
)

func mustPriv(seed byte) keys.Private {
	b64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 32))
	p, err := keys.ParsePrivate(b64)
	if err != nil {
		log.Fatalf("seed key: %v", err)
	}
	return p
}

func main() {
	id := flag.String("id", "a", "a or b")
	flag.Parse()

	privA, privB := mustPriv(1), mustPriv(2)

	var priv keys.Private
	var peerPubHex, allowed string
	var listen, peer int
	switch *id {
	case "a":
		priv, peerPubHex = privA, privB.Public().Hex()
		listen, peer, allowed = 51830, 51831, "100.64.0.2/32"
	case "b":
		priv, peerPubHex = privB, privA.Public().Hex()
		listen, peer, allowed = 51831, 51830, "100.64.0.1/32"
	default:
		log.Fatal("--id must be a or b")
	}

	ch := tuntest.NewChannelTUN()
	ad := &tun.Adapter{Name: "probe-" + *id, Dev: ch.TUN()}
	dev, err := wg.Up(ad, priv, listen)
	if err != nil {
		log.Fatalf("wg up: %v", err)
	}
	defer dev.Close()

	if err := dev.SetPeers([]wg.Peer{{
		PublicKeyHex: peerPubHex,
		Endpoint:     fmt.Sprintf("127.0.0.1:%d", peer),
		AllowedIP:    allowed,
	}}); err != nil {
		log.Fatalf("set peers: %v", err)
	}
	log.Printf("[%s] up: listen=%d peer=127.0.0.1:%d", *id, listen, peer)

	for i := 0; i < 20; i++ {
		hs, _ := dev.PeerHandshakes()
		if hs[peerPubHex] > 0 {
			log.Printf("[%s] handshake OK", *id)
			return
		}
		log.Printf("[%s] awaiting handshake ...", *id)
		time.Sleep(1 * time.Second)
	}
	log.Printf("[%s] NO handshake after 20s", *id)
}
