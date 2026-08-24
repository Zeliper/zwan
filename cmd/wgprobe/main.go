// Command wgprobe is a diagnostic: one wireguard-go device on an in-memory
// channel TUN (no admin / Wintun), talking to another wgprobe.
//
// Direct mode (default): the two probes send WireGuard straight to each other
// over loopback UDP. Relay mode (--relay host:port): they tunnel through the
// server relay, exactly as the agent's --relay path does. Either way, run two
// processes (--id a and --id b). Set ZWAN_WG_LOG=verbose for WG internals.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"time"

	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/client/wg"
	"github.com/Zeliper/zwan/client/wgbind"
	"github.com/Zeliper/zwan/shared/keys"
	"golang.zx2c4.com/wireguard/conn"
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
	relayAddr := flag.String("relay", "", "relay host:port (empty = direct loopback)")
	flag.Parse()

	privA, privB := mustPriv(1), mustPriv(2)

	var priv keys.Private
	var peerPubHex, selfIP, peerIP string
	var listen, peer int
	switch *id {
	case "a":
		priv, peerPubHex = privA, privB.Public().Hex()
		selfIP, peerIP, listen, peer = "100.64.0.1", "100.64.0.2", 51830, 51831
	case "b":
		priv, peerPubHex = privB, privA.Public().Hex()
		selfIP, peerIP, listen, peer = "100.64.0.2", "100.64.0.1", 51831, 51830
	default:
		log.Fatal("--id must be a or b")
	}

	var bind conn.Bind
	var endpoint string
	if *relayAddr != "" {
		b, err := wgbind.NewRelay(*relayAddr, netip.MustParseAddr(selfIP))
		if err != nil {
			log.Fatalf("relay bind: %v", err)
		}
		bind, endpoint = b, peerIP // relay routes by overlay IP
	} else {
		bind, endpoint = wgbind.New(), fmt.Sprintf("127.0.0.1:%d", peer)
	}

	ch := tuntest.NewChannelTUN()
	ad := &tun.Adapter{Name: "probe-" + *id, Dev: ch.TUN()}
	dev, err := wg.Up(ad, priv, listen, bind)
	if err != nil {
		log.Fatalf("wg up: %v", err)
	}
	defer dev.Close()

	if err := dev.SetPeers([]wg.Peer{{
		PublicKeyHex: peerPubHex,
		Endpoint:     endpoint,
		AllowedIP:    peerIP + "/32",
	}}); err != nil {
		log.Fatalf("set peers: %v", err)
	}
	log.Printf("[%s] up: self=%s peer=%s endpoint=%s relay=%q", *id, selfIP, peerIP, endpoint, *relayAddr)

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
