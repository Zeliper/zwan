//go:build !windows

package nrpt

// Supported reports whether this system has a name resolution policy table.
// Windows is the only one, so elsewhere the binder installs nothing and reports
// nothing installed — callers need no build tags of their own, but should check
// this before telling the operator that names resolve system-wide.
const Supported = false

func list() ([]Rule, error) { return nil, nil }

func mutate(add, remove []string, server string) error { return nil }
