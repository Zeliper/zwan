//go:build !windows

package wgbind

import "syscall"

// controlDisableConnReset is a no-op off Windows (SIO_UDP_CONNRESET is Windows-only).
func controlDisableConnReset(network, address string, c syscall.RawConn) error { return nil }

func isConnReset(err error) bool { return false }
