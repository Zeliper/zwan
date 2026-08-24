// Command zwan-server is the self-hosted control plane.
//
// M0 skeleton: exposes a minimal health endpoint. Real control-plane services
// (auth, IPAM, DNS, service registry, ACL, STUN, relay) arrive in later milestones.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/Zeliper/zwan/shared"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (plain HTTP loopback for M0; TLS/ACME on :443 in M5)")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"product":   shared.ProductName,
			"component": string(shared.ComponentServer),
			"version":   shared.Version,
		})
	})

	log.Printf("%s (%s) %s listening on %s", shared.ProductName, shared.ComponentServer, shared.Version, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
