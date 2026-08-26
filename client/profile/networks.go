package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/proto"
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

	// Publish are the services this device offers on the network. They are
	// remembered here rather than on the server: the server's registry lives in
	// memory, so this is what puts them back after either end restarts.
	Publish []proto.Service `json:"publish,omitempty"`
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
	seen := make(map[string]bool, len(n.Publish))
	for _, s := range n.Publish {
		if err := ValidService(s); err != nil {
			return err
		}
		name := strings.ToLower(strings.TrimSpace(s.Name))
		if seen[name] {
			return fmt.Errorf("two services are both called %q", s.Name)
		}
		seen[name] = true
	}
	return nil
}

// ValidService reports why a service cannot be published, or nil.
//
// The name becomes a DNS label under the network's local name, so it has to
// survive being one; the port is what clients connect to, and the backend port
// is where this device forwards them. A backend of zero is not a mistake: it
// means the program binds the service's own overlay address itself, with no
// proxy in between.
func ValidService(s proto.Service) error {
	name := strings.ToLower(strings.TrimSpace(s.Name))
	if name == "" {
		return errors.New("a service needs a name")
	}
	if len(name) > 63 {
		return fmt.Errorf("service name %q is too long", s.Name)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(name)-1:
		default:
			return fmt.Errorf("service name %q may use letters, digits and inner hyphens only", s.Name)
		}
	}
	switch strings.ToLower(strings.TrimSpace(s.Proto)) {
	case "", "tcp", "udp":
	default:
		return fmt.Errorf("service %q: protocol must be tcp or udp", s.Name)
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("service %q needs a port between 1 and 65535", s.Name)
	}
	if s.BackendPort < 0 || s.BackendPort > 65535 {
		return fmt.Errorf("service %q: backend port must be between 1 and 65535, or empty", s.Name)
	}
	return nil
}

// NormalizePublish puts the published services into the form the rest of the
// code expects, so the UI does not have to.
func NormalizePublish(list []proto.Service) []proto.Service {
	out := make([]proto.Service, 0, len(list))
	for _, s := range list {
		s.Name = strings.ToLower(strings.TrimSpace(s.Name))
		s.Proto = strings.ToLower(strings.TrimSpace(s.Proto))
		if s.Proto == "" {
			s.Proto = "tcp"
		}
		groups := make([]string, 0, len(s.AllowGroups))
		for _, g := range s.AllowGroups {
			if g = strings.TrimSpace(g); g != "" {
				groups = append(groups, g)
			}
		}
		s.AllowGroups = groups
		// NodeIP and VIP are the server's to decide; whatever a caller put there
		// is stale the moment the address is reassigned.
		s.NodeIP, s.VIP = "", ""
		out = append(out, s)
	}
	return out
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
