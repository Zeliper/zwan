// Command zwan-agent is the Windows client that joins one or more networks.
//
// M1a: performs the control-plane join (register + peer list) and prints the
// result. The Wintun adapter, wireguard-go tunnel, split DNS, L4 service router
// and multi-network profile management arrive in later milestones.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/shared"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:8787", "control server URL")
	token := flag.String("token", "", "join token")
	device := flag.String("device", "", "stable device UUID")
	name := flag.String("name", "", "hostname label (defaults to OS hostname)")
	flag.Parse()

	log.Printf("%s (%s) %s", shared.ProductName, shared.ComponentAgent, shared.Version)
	if *token == "" || *device == "" {
		log.Fatal("--token and --device are required")
	}

	hostname := *name
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	res, err := join.Do(*server, *token, *device, hostname)
	if err != nil {
		log.Fatalf("join failed: %v", err)
	}

	log.Printf("joined: network=%s suffix=%s assigned_ip=%s",
		res.Register.NetworkID, res.Register.DNSSuffix, res.Register.AssignedIP)
	log.Printf("node public key: %s", res.PublicKey)
	log.Printf("peers (%d):", len(res.Peers))
	for _, p := range res.Peers {
		log.Printf("  %-15s %-12s %s", p.AssignedIP, p.Hostname, p.PublicKey)
	}
	// TODO(M1b): create Wintun adapter for assigned_ip and open wireguard-go
	// tunnels to peers (relay first, then P2P in M3).
}
