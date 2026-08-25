// Command zwan-agent is the CLI client that joins a network.
//
// Modes:
//
//	(default)   control-plane join only (register + peer list).
//	--tun       also create the adapter and assign the overlay IP.
//	--up        also run the full data plane: tunnel, routes, DNS, service proxies.
//
// --tun and --up require Administrator.
//
// The --up path runs client/engine, the same code the Windows service runs, so
// what the CLI does and what the service does cannot drift apart.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/join"
	"github.com/Zeliper/zwan/client/nrpt"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
)

func main() {
	server := flag.String("server", "https://127.0.0.1:8787", "control server URL (may carry the key pin as \"#sha256:...\")")
	pin := flag.String("pin", "", "control server key fingerprint; required when the server has no CA-issued certificate")
	token := flag.String("token", "", "join token")
	device := flag.String("device", "", "stable device UUID")
	name := flag.String("name", "", "hostname label (defaults to OS hostname)")
	useTun := flag.Bool("tun", false, "create the adapter and assign the overlay IP (Administrator)")
	useUp := flag.Bool("up", false, "run the full data plane: adapter, tunnel, routes, DNS and service proxies (Administrator)")
	useRelay := flag.Bool("relay", false, "route the tunnel through the server relay instead of direct endpoints")
	adapter := flag.String("adapter", "", "adapter name (default \"<Product>-<network>\")")
	wgPort := flag.Int("wg-port", 51820, "local WireGuard UDP listen port (direct mode)")
	endpoint := flag.String("endpoint", "", "endpoint host:port peers use to reach us (default 127.0.0.1:<wg-port>)")
	dnsAddr := flag.String("dns-addr", "127.0.0.1:53", "local DNS resolver listen address (with --up; empty to disable)")
	publishName := flag.String("publish-name", "", "publish a service of this name hosted on this node")
	publishPort := flag.Int("publish-port", 0, "published service port (over the overlay)")
	publishProto := flag.String("publish-proto", "tcp", "published service protocol (tcp/udp)")
	publishBackend := flag.Int("publish-backend-port", 0, "localhost backend port; enables the L4 proxy (0 = service binds the overlay itself)")
	publishAllow := flag.String("publish-allow", "", "comma-separated groups allowed to use the published service (default: everyone who can reach this node)")
	flag.Parse()

	log.Printf("%s (%s) %s", shared.ProductName, shared.ComponentAgent, shared.Version)
	if *token == "" || *device == "" {
		log.Fatal("--token and --device are required")
	}
	hostname := *name
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	ep := *endpoint
	if ep == "" {
		ep = fmt.Sprintf("127.0.0.1:%d", *wgPort)
	}

	var publish []proto.Service
	if *publishName != "" {
		if *publishPort == 0 {
			log.Fatal("--publish-port is required with --publish-name")
		}
		publish = append(publish, proto.Service{
			Name:        *publishName,
			Proto:       *publishProto,
			Port:        *publishPort,
			BackendPort: *publishBackend,
			AllowGroups: splitGroups(*publishAllow),
		})
	}

	if *useUp {
		runEngine(engine.Config{
			Server:      *server,
			Pin:         *pin,
			Token:       *token,
			DeviceUUID:  *device,
			Name:        hostname,
			UseRelay:    *useRelay,
			AdapterName: *adapter,
			WGPort:      *wgPort,
			Endpoint:    ep,
			DNSAddr:     *dnsAddr,
			ProductName: shared.ProductName,
			Publish:     publish,
		})
		return
	}

	// Control-plane modes do their own join: there is no engine to do it.
	cl, err := join.NewClient(*server, *pin)
	if err != nil {
		log.Fatalf("control server: %v", err)
	}
	log.Printf("control server: %s (%s)", cl.BaseURL(), trustLabel(cl))
	res, err := cl.Join(*token, *device, hostname, ep)
	if err != nil {
		log.Fatalf("join failed: %v", err)
	}
	log.Printf("joined: network=%s suffix=%s assigned_ip=%s",
		res.Register.NetworkID, res.Register.DNSSuffix, res.Register.AssignedIP)
	log.Printf("node public key: %s", res.PublicKey)
	printPeers(res.Peers)

	for _, svc := range publish {
		svc.NodeIP = res.Register.AssignedIP
		stored, err := cl.PublishService(svc)
		if err != nil {
			log.Fatalf("publish service: %v", err)
		}
		addr := stored.VIP
		if addr == "" {
			addr = stored.NodeIP
		}
		log.Printf("published service %s.%s at %s:%d/%s (backend 127.0.0.1:%d, allowed: %s)",
			stored.Name, res.Register.DNSSuffix, addr, stored.Port, stored.Proto, stored.BackendPort, allowLabel(stored.AllowGroups))
	}

	if *useTun {
		bringUpAdapter(res, adapterName(*adapter, res.Register.NetworkID))
	}
}

// runEngine brings the whole data plane up and reports it until interrupted.
func runEngine(cfg engine.Config) {
	eng := engine.New()
	if err := eng.Start(cfg); err != nil {
		log.Fatalf("start: %v", err)
	}
	defer eng.Stop()

	st := eng.Status()
	log.Printf("control server: %s (%s)", st.Server, trustLabelOf(st))
	log.Printf("joined: network=%s suffix=%s assigned_ip=%s via %s",
		st.NetworkID, st.DNSSuffix, st.AssignedIP, st.Via)
	log.Printf("node public key: %s", st.PublicKey)
	defer bindSystemDNS(cfg.DNSAddr, st.DNSSuffix)()
	log.Print("engine up; Ctrl+C to stop")

	stop := make(chan struct{})
	go reportStatus(eng, stop)
	waitForInterrupt()
	close(stop)
}

// bindSystemDNS points the machine's name resolution at our resolver for this
// network's suffix, and returns the function that takes the rule back out.
//
// Without it the resolver answers only whoever queries it directly, so `ping
// minecraft.home` still goes to the public internet. The service does the same
// thing through client/manager; doing it here too is what makes a CLI run an
// end-to-end test of the whole name path.
func bindSystemDNS(dnsAddr, suffix string) func() {
	nothing := func() {}
	if dnsAddr == "" || suffix == "" {
		return nothing
	}
	if !nrpt.Supported {
		log.Printf("system DNS: Windows only; names resolve through %s only", dnsAddr)
		return nothing
	}
	b, err := nrpt.New(dnsAddr)
	if err != nil {
		log.Printf("system DNS: %v (names resolve through %s only)", err, dnsAddr)
		return nothing
	}
	if err := b.Apply([]string{suffix}); err != nil {
		log.Printf("system DNS: %v (names resolve through %s only)", err, dnsAddr)
		return nothing
	}
	log.Printf("system DNS: *.%s -> %s", suffix, dnsAddr)
	return func() {
		// A rule is machine state: it outlives this process, and left behind it
		// sends the whole suffix to a resolver that is no longer listening.
		if err := b.Clear(); err != nil {
			log.Printf("system DNS cleanup: %v", err)
		}
	}
}

// reportStatus mirrors the engine's view into the log, which is the only status
// surface a CLI run has. It prints on change rather than on every tick, so a
// steady tunnel stays quiet.
func reportStatus(eng *engine.Engine, stop <-chan struct{}) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	var lastPeers, lastErr string
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		st := eng.Status()

		lines := make([]string, 0, len(st.Peers))
		for _, p := range st.Peers {
			if p.AssignedIP == st.AssignedIP {
				continue
			}
			state := "connecting"
			if st.Handshakes[p.AssignedIP] {
				state = "handshake OK"
			}
			label := p.Hostname
			if p.Group != "" {
				label += " [" + p.Group + "]"
			}
			lines = append(lines, fmt.Sprintf("%s %s (%s)", p.AssignedIP, label, state))
		}
		sort.Strings(lines)

		if summary := strings.Join(lines, " | "); summary != lastPeers {
			lastPeers = summary
			if summary == "" {
				log.Print("peers: none")
			} else {
				log.Printf("peers: %s", summary)
			}
		}
		if st.LastError != "" && st.LastError != lastErr {
			lastErr = st.LastError
			log.Printf("engine: %s", st.LastError)
		}
	}
}

func printPeers(peers []proto.Peer) {
	log.Printf("peers (%d):", len(peers))
	for _, p := range peers {
		log.Printf("  %-15s %-12s %-8s ep=%-21s %s", p.AssignedIP, p.Hostname, p.Group, p.Endpoint, p.PublicKey)
	}
}

// bringUpAdapter creates the adapter and assigns the node IP only, with no
// tunnel — enough to check that Wintun and the address plumbing work.
func bringUpAdapter(res *join.Result, name string) {
	log.Printf("creating adapter %q ...", name)
	ad, err := tun.Create(name, tun.DefaultMTU)
	if err != nil {
		log.Fatalf("adapter: %v (Administrator? wintun.dll present?)", err)
	}
	defer func() {
		log.Printf("removing adapter %q", name)
		_ = ad.Close()
	}()

	if err := ad.SetNodeIP(res.Register.AssignedIP); err != nil {
		log.Fatalf("assign overlay IP: %v", err)
	}
	log.Printf("adapter %q up with %s. Ctrl+C to remove.", name, res.Register.AssignedIP)
	waitForInterrupt()
}

func adapterName(override, networkID string) string {
	if override != "" {
		return override
	}
	return fmt.Sprintf("%s-%s", shared.ProductName, networkID)
}

// trustLabel describes how the control server's identity was verified.
func trustLabel(cl *join.Client) string {
	switch {
	case strings.HasPrefix(cl.BaseURL(), "http://"):
		return "plaintext - no server authentication"
	case cl.Pin() != "":
		return "TLS, pinned " + cl.Pin()
	default:
		return "TLS, CA-verified"
	}
}

func trustLabelOf(st engine.Status) string {
	switch {
	case strings.HasPrefix(st.Server, "http://"):
		return "plaintext - no server authentication"
	case st.Pinned:
		return "TLS, pinned"
	default:
		return "TLS, CA-verified"
	}
}

// splitGroups parses the comma-separated --publish-allow value.
func splitGroups(s string) []string {
	var out []string
	for _, g := range strings.Split(s, ",") {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

// allowLabel renders a service's allow list for the log line.
func allowLabel(groups []string) string {
	if len(groups) == 0 {
		return "any group that can reach this node"
	}
	return strings.Join(groups, ",")
}

func waitForInterrupt() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
