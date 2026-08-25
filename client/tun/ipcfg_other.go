//go:build !windows

package tun

import (
	"errors"
	"runtime"
)

// setIPv4 is not implemented off Windows yet (the agent targets Windows).
func setIPv4(name, ip string, prefix int) error {
	return errors.New("tun: IPv4 assignment not implemented on " + runtime.GOOS)
}

// addIPv4 is not implemented off Windows yet.
func addIPv4(name, ip string, prefix int) error {
	return errors.New("tun: IPv4 assignment not implemented on " + runtime.GOOS)
}

// addRoute is not implemented off Windows yet.
func addRoute(name, peerIP string) error {
	return errors.New("tun: route management not implemented on " + runtime.GOOS)
}
