//go:build !windows

package firewall

import "errors"

// Supported reports whether rules can be managed on this system. Only Windows
// hides an inbound block behind a prompt that never appears; elsewhere the
// operator's own firewall is theirs to configure.
const Supported = false

func Allow(r Rule) error { return errors.New("firewall rules are managed only on Windows") }

func Remove(name string) error { return nil }
