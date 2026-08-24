// Command zwan-agent is the Windows client that joins one or more networks.
//
// M1a: control-plane join (register + peer list).
// M1b-1: with --tun, also create the Wintun adapter and assign the overlay IP
// (requires Administrator). The wireguard-go tunnel + relay arrive in M1b-2.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:8787", "control server URL")
	token := flag.String("token", "", "join token")
	device := flag.String("device", "", "stable device UUID")
	name := flag.String("name", "", "hostname label (defaults to OS hostname)")
	useTun := flag.Bool("tun", false, "create the virtual adapter and assign the overlay IP (requires Administrator)")
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

	if *useTun {
		bringUpAdapter(res)
	}
	// TODO(M1b-2): start a wireguard-go device on this adapter and open tunnels
	// to peers through the server relay (P2P path added in M3).
}

func bringUpAdapter(res *join.Result) {
	adapterName := fmt.Sprintf("%s-%s", shared.ProductName, res.Register.NetworkID) // e.g. "MyWAN-demo"

	log.Printf("creating adapter %q ...", adapterName)
	ad, err := tun.Create(adapterName, tun.DefaultMTU)
	if err != nil {
		log.Fatalf("adapter: %v (are you running as Administrator? is wintun.dll present?)", err)
	}
	defer func() {
		log.Printf("removing adapter %q", adapterName)
		_ = ad.Close()
	}()

	if err := ad.SetOverlayIP(res.Register.AssignedIP, res.Register.OverlayCIDR); err != nil {
		log.Fatalf("assign overlay IP: %v", err)
	}
	log.Printf("adapter %q is up with %s (%s)", adapterName, res.Register.AssignedIP, res.Register.OverlayCIDR)
	log.Printf("try: ping %s   (from this or another shell). Ctrl+C to remove the adapter and exit.", res.Register.AssignedIP)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
