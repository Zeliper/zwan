package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zeliper/zwan/shared"
)

// NetworksFile is the file name inside the machine state directory.
const NetworksFile = "networks.json"

// Network is one joined network as this device remembers it.
//
// Alias is the device's own short name for the network and doubles as the local
// DNS suffix. Two servers can advertise the same suffix, so the name a device
// uses has to be chosen locally to stay unique (design doc 47.1).
type Network struct {
	Alias       string `json:"alias"`
	Server      string `json:"server"`
	Pin         string `json:"pin,omitempty"`
	Token       string `json:"token"`
	Name        string `json:"name,omitempty"` // hostname label to register with
	UseRelay    bool   `json:"useRelay"`
	AutoConnect bool   `json:"autoConnect"`
}

type networksFile struct {
	Networks []Network `json:"networks"`
}

// NetworksPath is where the joined networks are stored.
func NetworksPath() (string, error) {
	dir, err := shared.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, NetworksFile), nil
}

// LoadNetworks reads the joined networks, returning none when nothing has been
// saved yet.
func LoadNetworks() ([]Network, error) {
	path, err := NetworksPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f networksFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return f.Networks, nil
}

// SaveNetworks writes the joined networks. They hold join tokens, so the file is
// created private to its owner.
func SaveNetworks(nets []Network) error {
	path, err := NetworksPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(networksFile{Networks: nets}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// Validate checks a network before it is stored or connected.
func (n Network) Validate() error {
	if err := ValidAlias(n.Alias); err != nil {
		return err
	}
	if strings.TrimSpace(n.Server) == "" {
		return errors.New("a server address is required")
	}
	if strings.TrimSpace(n.Token) == "" {
		return errors.New("a join token is required")
	}
	return nil
}

// NormalizeAlias lowercases and trims an alias to its canonical form.
func NormalizeAlias(alias string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(alias), "."))
}

// ValidAlias reports whether an alias can be used as a DNS suffix. It is what
// the user types after a service name, so it has to survive the resolver and
// Windows' NRPT rules unchanged.
func ValidAlias(alias string) error {
	a := NormalizeAlias(alias)
	if a == "" {
		return errors.New("a local name is required")
	}
	if len(a) > 253 {
		return errors.New("local name is too long")
	}
	for _, label := range strings.Split(a, ".") {
		if label == "" {
			return fmt.Errorf("local name %q has an empty part", alias)
		}
		if len(label) > 63 {
			return fmt.Errorf("local name part %q is too long", label)
		}
		for i, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			case r == '-' && i > 0 && i < len(label)-1:
			default:
				return fmt.Errorf("local name %q may use letters, digits and inner hyphens only", alias)
			}
		}
	}
	return nil
}
