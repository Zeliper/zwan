// Package profile persists small per-user client state, starting with a stable
// device identifier so a machine keeps its assigned overlay IP across restarts.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// DeviceUUID returns a stable per-user device identifier (under the OS user
// config directory), creating and persisting one on first use.
func DeviceUUID() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return deviceUUIDAt(filepath.Join(dir, "zwan"))
}

// MachineDeviceUUID returns a machine-wide device identifier (under ProgramData),
// used by the SYSTEM engine service so a machine keeps its overlay IP.
func MachineDeviceUUID() (string, error) {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return deviceUUIDAt(filepath.Join(base, "zwan"))
}

func deviceUUIDAt(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "device_id")
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); len(id) >= 8 {
			return id, nil
		}
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		return "", err
	}
	return id, nil
}
