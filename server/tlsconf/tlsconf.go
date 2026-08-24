// Package tlsconf builds the TLS configuration for the control plane.
//
// Two trust models are supported, matching the design's trust model (doc 36 and
// 47.1): a server that owns a public domain gets a CA-issued certificate
// automatically over ACME, and a server reached by bare IP falls back to a
// persistent self-signed certificate whose public-key fingerprint (pin) clients
// verify instead of a chain.
package tlsconf

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Mode selects how the control plane presents itself.
type Mode string

const (
	// ModeAuto picks ACME when domains are configured and self-signed otherwise.
	ModeAuto Mode = "auto"
	// ModeOff serves plaintext HTTP (local development, or behind a TLS proxy).
	ModeOff Mode = "off"
	// ModeSelf serves a persistent self-signed certificate verified by pin.
	ModeSelf Mode = "self"
	// ModeACME obtains a CA-issued certificate for the configured domains.
	ModeACME Mode = "acme"
)

// ParseMode validates a mode name.
func ParseMode(s string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case "", ModeAuto:
		return ModeAuto, nil
	case ModeOff:
		return ModeOff, nil
	case ModeSelf, "self-signed", "selfsigned":
		return ModeSelf, nil
	case ModeACME:
		return ModeACME, nil
	default:
		return "", fmt.Errorf("unknown TLS mode %q (want auto, off, self or acme)", s)
	}
}

// Config describes the desired TLS setup.
type Config struct {
	Mode      Mode     // default ModeAuto
	Domains   []string // public hostnames; required for ACME, also SANs for self-signed
	ExtraSANs []string // extra hostnames/IPs to put in the self-signed certificate
	Dir       string   // state directory for the key, certificate and ACME cache
	Email     string   // ACME account contact (optional)
	Directory string   // ACME directory URL override (e.g. Let's Encrypt staging)

	// ListenAddr is the control API listen address. It only decides whether an
	// HTTP-01 challenge listener is needed: TLS-ALPN-01 works when the API
	// itself is on :443, otherwise ACME must be answered over port 80.
	ListenAddr string
}

// Result is a built TLS setup.
type Result struct {
	Mode    Mode
	TLS     *tls.Config // nil when Mode is ModeOff
	Pin     string      // SPKI pin clients must verify; only set for ModeSelf
	Domains []string

	// HTTPChallenge is an ACME HTTP-01 handler that must be served on port 80
	// when the control API does not listen on 443. Nil when not needed.
	HTTPChallenge http.Handler
}

// Scheme is the URL scheme clients must use to reach this server.
func (r *Result) Scheme() string {
	if r.Mode == ModeOff {
		return "http"
	}
	return "https"
}

// Build resolves the mode and returns the TLS configuration to serve with.
func Build(cfg Config) (*Result, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = ModeAuto
	}
	domains := cleanHosts(cfg.Domains)
	if mode == ModeAuto {
		if len(domains) > 0 {
			mode = ModeACME
		} else {
			mode = ModeSelf
		}
	}

	switch mode {
	case ModeOff:
		return &Result{Mode: ModeOff}, nil

	case ModeSelf:
		if cfg.Dir == "" {
			return nil, fmt.Errorf("self-signed TLS needs a state directory")
		}
		cert, pin, err := selfSigned(cfg.Dir, selfSANs(domains, cfg.ExtraSANs))
		if err != nil {
			return nil, err
		}
		return &Result{
			Mode:    ModeSelf,
			Pin:     pin,
			Domains: domains,
			TLS: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cert},
			},
		}, nil

	case ModeACME:
		if len(domains) == 0 {
			return nil, fmt.Errorf("ACME needs at least one domain (--domain); use --tls=self for an IP-only server")
		}
		if cfg.Dir == "" {
			return nil, fmt.Errorf("ACME needs a state directory for its certificate cache")
		}
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cfg.Dir),
			HostPolicy: autocert.HostWhitelist(domains...),
			Email:      cfg.Email,
		}
		if cfg.Directory != "" {
			m.Client = &acme.Client{DirectoryURL: cfg.Directory}
		}
		tc := m.TLSConfig()
		tc.MinVersion = tls.VersionTLS12
		res := &Result{Mode: ModeACME, Domains: domains, TLS: tc}
		if !isPort(cfg.ListenAddr, "443") {
			// TLS-ALPN-01 is only reachable when the API itself serves :443.
			res.HTTPChallenge = m.HTTPHandler(nil)
		}
		return res, nil
	}
	return nil, fmt.Errorf("unknown TLS mode %q", mode)
}

// selfSANs is the SAN set for a self-signed certificate: loopback (so local
// tooling and the in-process GUI host work), the configured domains, any extra
// names, and the machine's own routable addresses so an IP-based join presents a
// matching certificate even to non-pinning clients.
func selfSANs(domains, extra []string) []string {
	sans := []string{"localhost", "127.0.0.1", "::1"}
	sans = append(sans, domains...)
	sans = append(sans, cleanHosts(extra)...)
	return append(sans, localAddrs()...)
}

func localAddrs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.IsLinkLocalMulticast() {
			continue
		}
		out = append(out, n.IP.String())
	}
	return out
}

func cleanHosts(in []string) []string {
	var out []string
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func isPort(addr, port string) bool {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return p == port
}
