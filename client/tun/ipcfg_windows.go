//go:build windows

package tun

import (
	"fmt"
	"net"
	"os/exec"
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
