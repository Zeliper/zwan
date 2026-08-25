// Package firewall opens the way in for a program Windows will never ask the
// user about.
//
// A service running as SYSTEM gets no "allow this app to communicate?" prompt:
// the prompt belongs to an interactive desktop, and there is none. Inbound
// packets are simply dropped, while the program itself is bound and healthy and
// says so. The only visible symptom is a client somewhere else timing out, which
// is a long way from the cause.
//
// The rules are scoped to the executable rather than to a port. Ports here are
// configuration — the control port, the relay port and the tunnel port all move
// — and a rule naming the wrong port is the same silence as no rule at all. The
// program is the thing that does not change.
package firewall

import "strings"

// Rule is one inbound allowance.
type Rule struct {
	// Name identifies the rule in the firewall, and is what removal matches on.
	Name string
	// Exe is the full path of the program allowed to accept connections.
	Exe string
	// Description is shown to an administrator reading the rule list, so it
	// should say why the rule exists rather than repeat the name.
	Description string
}

// addArgs builds the arguments that install a rule.
//
// profile=any is deliberate. A machine hosting a network is reached from the
// internet, and a home or café connection is usually classified Public — a rule
// limited to Private would be a rule that quietly does not apply on the network
// the user is actually on.
func addArgs(r Rule) []string {
	return []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + r.Name,
		"dir=in",
		"action=allow",
		"program=" + r.Exe,
		"description=" + r.Description,
		"profile=any",
		"enable=yes",
	}
}

// deleteArgs builds the arguments that remove every rule with this name.
func deleteArgs(name string) []string {
	return []string{"advfirewall", "firewall", "delete", "rule", "name=" + name}
}

// notFound reports whether a delete failed only because there was nothing to
// delete, which is the ordinary case on a first install and not an error.
// Windows localises the message, so the Korean phrasing is matched alongside.
func notFound(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "no rules match") ||
		strings.Contains(s, "not match") ||
		strings.Contains(s, "일치하는") ||
		strings.Contains(s, "찾을 수 없")
}
