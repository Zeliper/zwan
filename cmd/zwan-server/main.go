// Command zwan-server is the self-hosted control plane.
//
// M1a: serves the control-plane API (register + peers) for a single demo
// network held in memory. TLS/ACME, persistence, DNS, STUN and relay arrive
// in later milestones.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/Zeliper/zwan/server/api"
	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/relay"
	"github.com/Zeliper/zwan/server/store"
	"github.com/Zeliper/zwan/shared"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "control API listen address (plain HTTP for M0-M1b; TLS/ACME on :443 in M5)")
	netID := flag.String("network", "demo", "network id")
	suffix := flag.String("dns-suffix", "demo.zwan", "DNS suffix / namespace for this network")
	cidr := flag.String("cidr", "100.64.0.0/16", "overlay CIDR")
	token := flag.String("token", "", "join token (required)")
	relayAddr := flag.String("relay-addr", "127.0.0.1:3478", "relay UDP listen address")
	relayPublic := flag.String("relay-public", "", "relay host:port advertised to clients (default = relay-addr)")
	flag.Parse()

	if *token == "" {
		log.Fatal("--token is required")
	}

	relayPub := *relayPublic
	if relayPub == "" {
		relayPub = *relayAddr
	}

	alloc, err := ipam.New(*cidr)
	if err != nil {
		log.Fatalf("ipam: %v", err)
	}
	nw := store.NewNetwork(*netID, *suffix, *cidr)
	srv := api.New(nw, alloc, *token, relayPub)

	rly := relay.New()
	go func() {
		if err := rly.ListenAndServe(*relayAddr); err != nil {
			log.Fatalf("relay: %v", err)
		}
	}()

	log.Printf("%s (%s) %s: control on %s, relay on %s (public %s) (network=%s suffix=%s cidr=%s)",
		shared.ProductName, shared.ComponentServer, shared.Version, *addr, *relayAddr, relayPub, *netID, *suffix, *cidr)
	log.Fatal(http.ListenAndServe(*addr, srv.Routes()))
}
