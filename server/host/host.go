// Package host bundles the control API, IPAM, store and relay into one
// start/stoppable unit, so a server can run headless (cmd/zwan-server) or be
// hosted in-process by the desktop GUI.
package host

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/Zeliper/zwan/server/api"
	"github.com/Zeliper/zwan/server/ipam"
	"github.com/Zeliper/zwan/server/relay"
	"github.com/Zeliper/zwan/server/store"
)

// Config configures a hosted network.
type Config struct {
	NetworkID   string
	DNSSuffix   string
	CIDR        string
	Token       string
	ControlAddr string // control API listen (host:port)
	RelayAddr   string // relay UDP listen (host:port)
	RelayPublic string // relay host:port advertised to clients (default = RelayAddr)
}

// Host is a running server instance.
type Host struct {
	cfg     Config
	httpSrv *http.Server
	rly     *relay.Relay
}

// New returns an idle host.
func New() *Host { return &Host{} }

// Start brings the server up (control API + relay). It returns once both are
// listening; it serves in the background until Stop.
func (h *Host) Start(cfg Config) error {
	if h.httpSrv != nil {
		return errors.New("host already running")
	}
	relayPub := cfg.RelayPublic
	if relayPub == "" {
		relayPub = cfg.RelayAddr
	}
	alloc, err := ipam.New(cfg.CIDR)
	if err != nil {
		return err
	}
	nw := store.NewNetwork(cfg.NetworkID, cfg.DNSSuffix, cfg.CIDR)
	srv := api.New(nw, alloc, cfg.Token, relayPub)

	rly := relay.New()
	if _, err := rly.Listen(cfg.RelayAddr); err != nil {
		return err
	}
	go func() { _ = rly.Serve() }()

	ln, err := net.Listen("tcp", cfg.ControlAddr)
	if err != nil {
		_ = rly.Close()
		return err
	}
	httpSrv := &http.Server{Handler: srv.Routes()}
	go func() { _ = httpSrv.Serve(ln) }()

	h.cfg, h.httpSrv, h.rly = cfg, httpSrv, rly
	return nil
}

// Stop shuts the server down.
func (h *Host) Stop() {
	if h.httpSrv != nil {
		_ = h.httpSrv.Shutdown(context.Background())
		h.httpSrv = nil
	}
	if h.rly != nil {
		_ = h.rly.Close()
		h.rly = nil
	}
}

// Running reports whether the host is serving.
func (h *Host) Running() bool { return h.httpSrv != nil }

// Config returns the running configuration.
func (h *Host) Config() Config { return h.cfg }
