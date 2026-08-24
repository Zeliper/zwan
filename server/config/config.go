// Package config persists the control server's settings.
//
// The headless binary takes everything on the command line, but the Windows
// service is started by the SCM with no arguments, so the settings an operator
// entered in the GUI have to survive a reboot somewhere. That somewhere is a
// JSON file in the machine state directory.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/shared"
)

// FileName is the config file's name inside the machine state directory.
const FileName = "server.json"

// Config is a hosted network's saved settings.
type Config struct {
	NetworkID   string   `json:"networkId"`
	DNSSuffix   string   `json:"dnsSuffix"`
	CIDR        string   `json:"cidr"`
	Token       string   `json:"token"`
	ControlAddr string   `json:"controlAddr"`
	RelayAddr   string   `json:"relayAddr"`
	RelayPublic string   `json:"relayPublic"`
	TLSMode     string   `json:"tlsMode"`
	Domains     []string `json:"domains"`
	PublicHost  string   `json:"publicHost"`

	// AutoStart makes the service bring the network up on boot. It is false
	// until an operator has actually configured and started a network once.
	AutoStart bool `json:"autoStart"`
}

// Default is the configuration a fresh install starts from.
func Default() Config {
	return Config{
		NetworkID:   "home",
		DNSSuffix:   "home.zwan",
		CIDR:        "100.64.0.0/16",
		ControlAddr: "0.0.0.0:8787",
		RelayAddr:   "0.0.0.0:3478",
		TLSMode:     "auto",
	}
}

// Path is the config file's location.
func Path() (string, error) {
	dir, err := shared.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Load reads the saved configuration, falling back to Default when none has
// been written yet.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	c := Default()
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	return c, nil
}

// Save writes the configuration. It holds the join token, so the file is
// created private to its owner.
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// Valid reports why the configuration cannot be started, or nil.
func (c Config) Valid() error {
	switch {
	case strings.TrimSpace(c.Token) == "":
		return errors.New("a join token is required")
	case strings.TrimSpace(c.NetworkID) == "":
		return errors.New("a network id is required")
	case strings.TrimSpace(c.CIDR) == "":
		return errors.New("an overlay CIDR is required")
	case strings.TrimSpace(c.ControlAddr) == "":
		return errors.New("a control listen address is required")
	}
	return nil
}

// Host converts the saved settings into the runtime host configuration.
func (c Config) Host() host.Config {
	return host.Config{
		NetworkID:   c.NetworkID,
		DNSSuffix:   c.DNSSuffix,
		CIDR:        c.CIDR,
		Token:       c.Token,
		ControlAddr: c.ControlAddr,
		RelayAddr:   c.RelayAddr,
		RelayPublic: c.RelayPublic,
		TLSMode:     c.TLSMode,
		TLSDomains:  c.Domains,
	}
}
