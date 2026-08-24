//go:build !windows

package update

import "errors"

// Apply is Windows-only (the installer is a Windows executable).
func Apply(installerURL string) error {
	return errors.New("update: apply is Windows-only")
}
