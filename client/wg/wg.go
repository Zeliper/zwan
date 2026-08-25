// Package wg wraps a wireguard-go device running on a zwan virtual adapter.
//
// M1b-2: direct WireGuard tunnels between peers using endpoints exchanged via
// the control server. The relay bind (for peers that cannot reach each other
// directly) is added in M1b-3 / M3.
package wg

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared/keys"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
)

// Peer is one WireGuard peer configuration.
type Peer struct {
	PublicKeyHex string
	Endpoint     string   // host:port (may be empty)
	AllowedIPs   []string // the peer's node address, plus any service addresses it hosts
}

// Device is a running wireguard-go device bound to a virtual adapter.
type Device struct {
	dev *device.Device
}

// Up starts a wireguard-go device on the adapter with the given private key and
// UDP listen port, over the provided bind (direct or relay).
func Up(adapter *tun.Adapter, priv keys.Private, listenPort int, bind conn.Bind) (*Device, error) {
	level := device.LogLevelError
	if os.Getenv("ZWAN_WG_LOG") == "verbose" {
		level = device.LogLevelVerbose
	}
	logger := device.NewLogger(level, fmt.Sprintf("(%s) ", adapter.Name))
	dev := device.NewDevice(adapter.Dev, bind, logger)

	cfg := fmt.Sprintf("private_key=%s\nlisten_port=%d\n", priv.Hex(), listenPort)
	if err := dev.IpcSet(cfg); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure device: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("bring device up: %w", err)
	}
	return &Device{dev: dev}, nil
}

// SetPeers replaces the device's peer set.
func (d *Device) SetPeers(peers []Peer) error {
	return d.dev.IpcSet(buildPeerConfig(peers))
}

// PeerHandshakes returns each peer's last-handshake time (Unix seconds, 0 if the
// tunnel has not completed a handshake yet), keyed by hex public key.
func (d *Device) PeerHandshakes() (map[string]int64, error) {
	s, err := d.dev.IpcGet()
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	var cur string
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			cur = v
		case "last_handshake_time_sec":
			if cur != "" {
				n, _ := strconv.ParseInt(v, 10, 64)
				out[cur] = n
			}
		}
	}
	return out, nil
}

// Close shuts the device down.
func (d *Device) Close() {
	if d.dev != nil {
		d.dev.Close()
	}
}

// buildPeerConfig renders the wireguard-go UAPI string that replaces all peers.
func buildPeerConfig(peers []Peer) string {
	var b strings.Builder
	b.WriteString("replace_peers=true\n")
	for _, p := range peers {
		fmt.Fprintf(&b, "public_key=%s\n", p.PublicKeyHex)
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
		}
		for _, ip := range p.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", ip)
		}
		fmt.Fprintf(&b, "persistent_keepalive_interval=25\n")
	}
	return b.String()
}
