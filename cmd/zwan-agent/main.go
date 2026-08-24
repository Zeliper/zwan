// Command zwan-agent is the Windows client that joins one or more networks.
//
// M0 skeleton: prints identity and exits. Later milestones add the Wintun adapter,
// wireguard-go tunnel sessions, control-server registration, split DNS, the L4
// service router, and multi-network profile management.
package main

import (
	"log"

	"github.com/Zeliper/zwan/shared"
)

func main() {
	log.Printf("%s (%s) %s starting (skeleton)", shared.ProductName, shared.ComponentAgent, shared.Version)
	// TODO(M1): create Wintun adapter, start wireguard-go session, register with control server.
}
