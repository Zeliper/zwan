//go:build windows

package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

// Supported reports whether rules can be managed on this system.
const Supported = true

// Allow installs an inbound rule for r, replacing any rule of the same name.
//
// netsh adds rather than replaces, so a second install would leave two identical
// rules behind; removing first makes this safe to call again.
func Allow(r Rule) error {
	if r.Name == "" || r.Exe == "" {
		return fmt.Errorf("firewall rule needs a name and a program path")
	}
	_ = Remove(r.Name)
	if out, err := run(addArgs(r)); err != nil {
		return fmt.Errorf("add firewall rule %q: %v: %s", r.Name, err, out)
	}
	return nil
}

// Remove deletes the rule, and is not an error when there is none.
func Remove(name string) error {
	out, err := run(deleteArgs(name))
	if err != nil && !notFound(out) {
		return fmt.Errorf("remove firewall rule %q: %v: %s", name, err, out)
	}
	return nil
}

func run(args []string) (string, error) {
	out, err := exec.Command("netsh", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
