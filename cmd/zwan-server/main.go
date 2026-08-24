// Command zwan-server is the self-hosted control plane (headless).
//
// It runs the control API + relay for a single network. The desktop GUI can host
// the same server in-process (server/host); this binary is for VPS / headless use.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/shared"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "control API listen address (TLS/ACME on :443 in M5)")
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

	h := host.New()
	if err := h.Start(host.Config{
		NetworkID:   *netID,
		DNSSuffix:   *suffix,
		CIDR:        *cidr,
		Token:       *token,
		ControlAddr: *addr,
		RelayAddr:   *relayAddr,
		RelayPublic: *relayPublic,
	}); err != nil {
		log.Fatalf("start: %v", err)
	}

	log.Printf("%s (%s) %s: control on %s, relay on %s (network=%s suffix=%s cidr=%s)",
		shared.ProductName, shared.ComponentServer, shared.Version, *addr, *relayAddr, *netID, *suffix, *cidr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	h.Stop()
}
