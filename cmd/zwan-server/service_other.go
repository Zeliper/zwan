//go:build !windows

package main

import "fmt"

// runAsService is Windows-only; elsewhere the server runs headless, supervised
// by systemd or an equivalent.
func runAsService() bool { return false }

// handleServiceCommand reports the Windows-only nature of the service commands
// rather than silently ignoring them.
func handleServiceCommand(args []string) bool {
	if len(args) == 0 || args[0] != "service" {
		return false
	}
	fmt.Println("the service commands are Windows-only; on Linux run zwan-server under systemd")
	return true
}
