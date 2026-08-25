package shared

import (
	"os"
	"path/filepath"
	"runtime"
)

// StateDirEnv overrides the state directory when set.
const StateDirEnv = "ZWAN_STATE_DIR"

// StateDir returns (and creates) a machine-wide directory for persistent state
// such as TLS keys and certificates, joined with elem.
//
// Windows uses %ProgramData%\zwan so the SYSTEM service and an elevated GUI see
// the same state; elsewhere it prefers /var/lib/zwan and falls back to the user
// config directory when that is not writable (running unprivileged).
func StateDir(elem ...string) (string, error) {
	// An explicit override keeps tests, portable installs and side-by-side dev
	// runs off the machine-wide directory.
	if base := os.Getenv(StateDirEnv); base != "" {
		dir := filepath.Join(append([]string{base}, elem...)...)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	for _, base := range stateDirCandidates() {
		if base == "" {
			continue
		}
		dir := filepath.Join(append([]string{base, "zwan"}, elem...)...)
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return dir, nil
		}
	}
	// Last resort: alongside the working directory.
	dir := filepath.Join(append([]string{".zwan"}, elem...)...)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func stateDirCandidates() []string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		cfg, _ := os.UserConfigDir()
		return []string{pd, cfg}
	}
	cfg, _ := os.UserConfigDir()
	return []string{"/var/lib", cfg}
}
