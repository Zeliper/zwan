// Command zwan-server is the self-hosted control plane (headless).
//
// It runs the control API + relay for a single network. The desktop GUI can host
// the same server in-process (server/host); this binary is for VPS / headless use.
//
// Subcommand:  zwan-server update   -> self-update to the latest release and exit.
// Flag:        --auto-update        -> periodically self-update and restart.
package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/Zeliper/zwan/client/update"
	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/shared"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		runUpdate()
		return
	}

	addr := flag.String("addr", "127.0.0.1:8787", "control API listen address (use :443 with --domain for ACME)")
	netID := flag.String("network", "demo", "network id")
	suffix := flag.String("dns-suffix", "demo.zwan", "DNS suffix / namespace for this network")
	cidr := flag.String("cidr", "100.64.0.0/16", "overlay CIDR")
	token := flag.String("token", "", "join token (required)")
	relayAddr := flag.String("relay-addr", "127.0.0.1:3478", "relay UDP listen address")
	relayPublic := flag.String("relay-public", "", "relay host:port advertised to clients (default = relay-addr)")
	tlsMode := flag.String("tls", "auto", "TLS mode: auto (ACME with --domain, else self-signed), off, self, acme")
	domains := flag.String("domain", "", "comma-separated public hostnames for the ACME certificate")
	tlsSANs := flag.String("tls-san", "", "comma-separated extra hostnames/IPs for the self-signed certificate")
	tlsDir := flag.String("tls-dir", "", "directory for the TLS key, certificate and ACME cache (default: machine state dir)")
	acmeEmail := flag.String("acme-email", "", "contact address for the ACME account")
	acmeDirectory := flag.String("acme-directory", "", "ACME directory URL override (e.g. Let's Encrypt staging)")
	acmeHTTPAddr := flag.String("acme-http-addr", ":80", "HTTP-01 challenge listen address (used when --addr is not :443)")
	publicHost := flag.String("public-host", "", "host:port clients should connect to, for the printed join address")
	autoUpdate := flag.Bool("auto-update", false, "periodically self-update to the latest release and restart")
	updateEvery := flag.Duration("auto-update-interval", 6*time.Hour, "how often to check for updates with --auto-update")
	flag.Parse()

	if *token == "" {
		log.Fatal("--token is required")
	}

	h := host.New()
	if err := h.Start(host.Config{
		NetworkID:     *netID,
		DNSSuffix:     *suffix,
		CIDR:          *cidr,
		Token:         *token,
		ControlAddr:   *addr,
		RelayAddr:     *relayAddr,
		RelayPublic:   *relayPublic,
		TLSMode:       *tlsMode,
		TLSDomains:    splitList(*domains),
		TLSExtraSANs:  splitList(*tlsSANs),
		TLSDir:        *tlsDir,
		ACMEEmail:     *acmeEmail,
		ACMEDirectory: *acmeDirectory,
		ACMEHTTPAddr:  *acmeHTTPAddr,
	}); err != nil {
		log.Fatalf("start: %v", err)
	}

	log.Printf("%s (%s) %s: control on %s (%s), relay on %s (network=%s suffix=%s cidr=%s)",
		shared.ProductName, shared.ComponentServer, shared.Version, *addr, trustLabel(h), *relayAddr, *netID, *suffix, *cidr)
	log.Printf("clients join with: --server %q --token <token>", h.JoinURL(*publicHost))
	if h.TLSMode() == "off" {
		log.Print("WARNING: TLS is off - the join token and the whole control channel are sent in the clear")
	}

	if *autoUpdate {
		go autoUpdateLoop(*updateEvery)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	h.Stop()
}

// runUpdate performs a one-shot self-update.
func runUpdate() {
	ver, updated, err := update.SelfUpdateServer(shared.Version)
	if err != nil {
		log.Fatalf("update: %v", err)
	}
	if !updated {
		log.Printf("already up to date (latest %s)", ver)
		return
	}
	log.Printf("updated to %s — restart the server to run the new version", ver)
}

// autoUpdateLoop checks periodically and restarts into the new binary on update.
func autoUpdateLoop(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		ver, updated, err := update.SelfUpdateServer(shared.Version)
		if err != nil {
			log.Printf("auto-update: %v", err)
			continue
		}
		if updated {
			log.Printf("auto-update: updated to %s, restarting", ver)
			restartSelf()
		}
	}
}

func restartSelf() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("restart: %v", err)
		return
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Start(); err != nil {
		log.Printf("restart: %v", err)
		return
	}
	os.Exit(0)
}

// splitList parses a comma-separated flag value.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// trustLabel describes how clients verify this server's identity.
func trustLabel(h *host.Host) string {
	switch h.TLSMode() {
	case "off":
		return "plaintext HTTP"
	case "self":
		return "TLS, self-signed - clients must pin " + h.Pin()
	case "acme":
		return "TLS, ACME certificate"
	default:
		return h.TLSMode()
	}
}
