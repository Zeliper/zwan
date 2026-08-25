//go:build windows

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// setIPv4 assigns a static IPv4 address to the adapter via netsh.
//
// The adapter can take a moment to become configurable after creation, so this
// retries briefly. netsh avoids depending on the GPL-licensed winipcfg package
// (see THIRD_PARTY_NOTICES.md); a direct IP Helper API path can replace it later.
func setIPv4(name, ip string, prefix int) error {
	mask := net.CIDRMask(prefix, 32)
	dotted := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	var last error
	for i := 0; i < 12; i++ {
		out, err := exec.Command("netsh", "interface", "ipv4", "set", "address",
			"name="+name, "static", ip, dotted).CombinedOutput()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("netsh set address (attempt %d): %v: %s", i+1, err, string(out))
		time.Sleep(500 * time.Millisecond)
	}
	return last
}

// addIPv4 adds a secondary IPv4 address to the adapter.
//
// A node hosting several services holds one address per service, which is what
// lets two of them listen on the same port.
func addIPv4(name, ip string, prefix int) error {
	mask := net.CIDRMask(prefix, 32)
	dotted := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	out, err := exec.Command("netsh", "interface", "ipv4", "add", "address",
		"name="+name, ip, dotted).CombinedOutput()
	if err != nil {
		if alreadyThere(string(out)) {
			return nil
		}
		return fmt.Errorf("netsh add address %s on %s: %v: %s", ip, name, err, string(out))
	}
	return nil
}

// alreadyThere reports whether netsh refused because the object is already
// there, which is not a failure for our purposes. Windows localizes these
// messages, so the Korean phrasing is matched alongside the English.
func alreadyThere(out string) bool {
	return strings.Contains(out, "already exists") || strings.Contains(out, "이미 있")
}

// addRoute adds a /32 route for peerIP via the named adapter (on-link).
func addRoute(name, peerIP string) error {
	out, err := exec.Command("netsh", "interface", "ipv4", "add", "route",
		"prefix="+peerIP+"/32", "interface="+name, "store=active").CombinedOutput()
	if err != nil {
		// A duplicate route is not an error for our purposes.
		if alreadyThere(string(out)) {
			return nil
		}
		return fmt.Errorf("netsh add route %s/32 dev %s: %v: %s", peerIP, name, err, string(out))
	}
	return nil
}
