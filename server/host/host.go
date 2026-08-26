// Package host bundles the control API, IPAM, store and relay into one
// start/stoppable unit, so a server can run headless (cmd/zwan-server) or be
// hosted in-process by the desktop GUI.
package host

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"

	"github.com/Zeliper/zwan/server/api"
	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/relay"
	"github.com/Zeliper/zwan/server/store"
	"github.com/Zeliper/zwan/server/tlsconf"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/acl"
	"github.com/Zeliper/zwan/shared/proto"
)

// Config configures a hosted network.
type Config struct {
	NetworkID   string
	DNSSuffix   string
	CIDR        string
	Token       string // join token for the default group
	ControlAddr string // control API listen (host:port)
	RelayAddr   string // relay UDP listen (host:port)
	RelayPublic string // relay host:port advertised to clients (default: PublicHost + the relay's port)

	// PublicHost is where clients reach this server from outside. It also
	// decides where the relay is advertised, since a client that can reach the
	// control plane at a host can reach the relay there too.
	PublicHost string

	// TLS. Mode is "auto" (default), "off", "self" or "acme"; auto means ACME
	// when Domains is set and a pinned self-signed certificate otherwise.
	TLSMode       string
	TLSDomains    []string // public hostnames for ACME (also SANs when self-signed)
	TLSExtraSANs  []string // extra hostnames/IPs for the self-signed certificate
	TLSDir        string   // state dir for key/cert/ACME cache (default: machine state dir)
	ACMEEmail     string
	ACMEDirectory string // ACME directory URL override (e.g. Let's Encrypt staging)
	ACMEHTTPAddr  string // HTTP-01 challenge listen address (default ":80")

	// Access control. GroupTokens maps a group name to the join token that
	// admits members into it; ACL is the rule set between groups. With no extra
	// tokens and no rules every member reaches every other, which is what a
	// single-group network wants.
	GroupTokens map[string]string
	ACL         []acl.Rule
}

// Host is a running server instance.
type Host struct {
	cfg      Config
	ctrlAddr string // actual bound control address (resolves :0 and wildcards)
	relayPub string // relay address handed to clients (never a listen address)
	httpSrv  *http.Server
	acmeSrv  *http.Server
	rly      *relay.Relay
	tlsRes   *tlsconf.Result
	nw       *store.Network
}

// New returns an idle host.
func New() *Host { return &Host{} }

// Start brings the server up (control API + relay). It returns once both are
// listening; it serves in the background until Stop.
func (h *Host) Start(cfg Config) error {
	if h.httpSrv != nil {
		return errors.New("host already running")
	}
	mode, err := tlsconf.ParseMode(cfg.TLSMode)
	if err != nil {
		return err
	}
	dir := cfg.TLSDir
	if dir == "" && mode != tlsconf.ModeOff {
		if dir, err = shared.StateDir("tls"); err != nil {
			return fmt.Errorf("tls state dir: %w", err)
		}
	}
	tlsRes, err := tlsconf.Build(tlsconf.Config{
		Mode:       mode,
		Domains:    cfg.TLSDomains,
		ExtraSANs:  cfg.TLSExtraSANs,
		Dir:        dir,
		Email:      cfg.ACMEEmail,
		Directory:  cfg.ACMEDirectory,
		ListenAddr: cfg.ControlAddr,
	})
	if err != nil {
		return err
	}

	nodes, services, err := ipam.Split(cfg.CIDR)
	if err != nil {
		return err
	}
	tokens, err := acl.BuildJoinTokens(cfg.Token, cfg.GroupTokens)
	if err != nil {
		return err
	}
	rly := relay.New()
	bound, err := rly.Listen(cfg.RelayAddr)
	if err != nil {
		return err
	}
	go func() { _ = rly.Serve() }()

	relayPub := relayPublic(cfg, bound.String())
	nw := store.NewNetwork(cfg.NetworkID, cfg.DNSSuffix, cfg.CIDR)
	srv := api.New(api.Options{
		Network: nw, Nodes: nodes, Services: services,
		Tokens: tokens, Policy: &acl.Policy{Rules: cfg.ACL}, RelayAddr: relayPub,
	})

	ln, err := net.Listen("tcp", cfg.ControlAddr)
	if err != nil {
		_ = rly.Close()
		return err
	}
	if tlsRes.TLS != nil {
		ln = tls.NewListener(ln, tlsRes.TLS)
	}
	httpSrv := &http.Server{Handler: srv.Routes()}
	go func() { _ = httpSrv.Serve(ln) }()

	var acmeSrv *http.Server
	if tlsRes.HTTPChallenge != nil {
		addr := cfg.ACMEHTTPAddr
		if addr == "" {
			addr = ":80"
		}
		acmeSrv = &http.Server{Addr: addr, Handler: tlsRes.HTTPChallenge}
		go func() {
			if err := acmeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("host: ACME HTTP-01 listener on %s: %v (certificate issuance may fail)", addr, err)
			}
		}()
	}

	h.cfg, h.ctrlAddr, h.relayPub = cfg, ln.Addr().String(), relayPub
	h.httpSrv, h.acmeSrv, h.rly, h.tlsRes, h.nw = httpSrv, acmeSrv, rly, tlsRes, nw
	return nil
}

// Stop shuts the server down.
func (h *Host) Stop() {
	if h.httpSrv != nil {
		_ = h.httpSrv.Shutdown(context.Background())
		h.httpSrv = nil
	}
	if h.acmeSrv != nil {
		_ = h.acmeSrv.Shutdown(context.Background())
		h.acmeSrv = nil
	}
	if h.rly != nil {
		_ = h.rly.Close()
		h.rly = nil
	}
	h.tlsRes = nil
	h.nw = nil
	h.ctrlAddr = ""
}

// Running reports whether the host is serving.
func (h *Host) Running() bool { return h.httpSrv != nil }

// Config returns the running configuration.
func (h *Host) Config() Config { return h.cfg }

// Members lists the network's registered devices. The server reads its own
// store directly rather than calling its API, which would need a member
// credential the server does not hold.
func (h *Host) Members() []proto.Peer {
	if h.nw == nil {
		return nil
	}
	members := h.nw.Members()
	sort.Slice(members, func(i, j int) bool { return members[i].AssignedIP < members[j].AssignedIP })
	out := make([]proto.Peer, 0, len(members))
	for _, m := range members {
		out = append(out, proto.Peer{
			Hostname: m.Hostname, PublicKey: m.PublicKey, AssignedIP: m.AssignedIP, Endpoint: m.Endpoint,
		})
	}
	return out
}

// Services lists the network's published services.
func (h *Host) Services() []proto.Service {
	if h.nw == nil {
		return nil
	}
	svcs := h.nw.Services()
	out := make([]proto.Service, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, proto.Service{
			Name: s.Name, Proto: s.Proto, Port: s.Port, BackendPort: s.BackendPort, NodeIP: s.NodeIP,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// TLSMode returns the resolved TLS mode ("off", "self" or "acme").
func (h *Host) TLSMode() string {
	if h.tlsRes == nil {
		return ""
	}
	return string(h.tlsRes.Mode)
}

// Pin returns the SPKI fingerprint clients must pin, or "" when the server
// presents a CA-issued certificate (or none at all).
func (h *Host) Pin() string {
	if h.tlsRes == nil {
		return ""
	}
	return h.tlsRes.Pin
}

// RelayPublic is the relay address clients are given. It is worth showing: an
// operator can read the listen address off their own configuration, but not what
// the far end will be told to send to.
func (h *Host) RelayPublic() string { return h.relayPub }

// Scheme is the URL scheme clients must use ("https", or "http" with TLS off).
func (h *Host) Scheme() string {
	if h.tlsRes == nil {
		return "http"
	}
	return h.tlsRes.Scheme()
}

// LocalURL is the base URL for querying this server from the same machine.
func (h *Host) LocalURL() string {
	if !h.Running() {
		return ""
	}
	return h.Scheme() + "://" + localAddr(h.ctrlAddr)
}

// JoinURL is the address to hand to clients: the first configured domain when
// there is one, otherwise the listen address, with the pin appended as a
// fragment so the whole thing can be copied as a single value.
func (h *Host) JoinURL(publicHost string) string {
	if h.tlsRes == nil {
		return ""
	}
	hostport := publicHost
	if hostport == "" {
		if len(h.tlsRes.Domains) > 0 {
			hostport = withPort(h.tlsRes.Domains[0], h.ctrlAddr)
		} else {
			hostport = localAddr(h.ctrlAddr)
		}
	} else {
		// An operator who types only a host means "reach me here", not "reach me
		// on 443". Without the port the address still looks right and still
		// parses, and the client quietly connects to whatever else answers on
		// 443 — which is a confusing way to find out, because what it reports is
		// a key that does not match the pin.
		hostport = ensurePort(hostport, h.ctrlAddr)
	}
	u := h.Scheme() + "://" + hostport
	if h.tlsRes.Pin != "" {
		u += "#" + url.PathEscape(h.tlsRes.Pin)
	}
	return u
}

// relayPublic decides where clients should send their tunnel traffic.
//
// A listen address is not a destination. "0.0.0.0:3478" means "every interface
// on this machine", and handing that to a client tells it to send the tunnel
// nowhere — while the join still succeeds and the network still reports itself
// connected, because none of that touches the relay.
//
// What the operator published wins. Failing that the relay is at the same host
// clients already reach the control plane at, on the relay's own port: anyone
// who got here can get there. Failing that too, a wildcard collapses to
// loopback, which is right for a local test and visibly wrong for anything else.
func relayPublic(cfg Config, bound string) string {
	if cfg.RelayPublic != "" {
		return ensurePort(cfg.RelayPublic, bound)
	}
	if host := hostOf(cfg.PublicHost); host != "" {
		if _, port, err := net.SplitHostPort(bound); err == nil && port != "" {
			return net.JoinHostPort(host, port)
		}
	}
	return localAddr(bound)
}

// hostOf takes the host out of an address that may or may not name a port.
func hostOf(hostport string) string {
	if hostport == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return hostport
}

// ensurePort leaves an address that already names a port alone, and gives the
// listening port to one that does not. An unbracketed IPv6 literal has no port
// by definition, and comes back bracketed.
func ensurePort(hostport, listenAddr string) string {
	if _, _, err := net.SplitHostPort(hostport); err == nil {
		return hostport
	}
	return withPort(hostport, listenAddr)
}

// withPort attaches the port from listenAddr to a bare hostname.
func withPort(hostname, listenAddr string) string {
	if _, p, err := net.SplitHostPort(listenAddr); err == nil && p != "" && p != "443" {
		return net.JoinHostPort(hostname, p)
	}
	return hostname
}

// localAddr rewrites a wildcard listen address to loopback for local queries.
func localAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
