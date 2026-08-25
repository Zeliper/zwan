//go:build windows

package nrpt

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Zeliper/zwan/shared"
)

// Supported reports whether this system has a name resolution policy table.
const Supported = true

// list reports the rules this program installed, and only those. The table is
// machine-wide: a domain policy or another program may have rules in it, and
// those are none of our business.
func list() ([]Rule, error) {
	out, err := powershell("Get-DnsClientNrptRule -ErrorAction Stop | " +
		"Where-Object { $_.Comment -eq '" + Tag + "' } | " +
		`ForEach-Object { "$($_.Name)|$($_.Namespace -join ',')|$($_.NameServers -join ',')" }`)
	if err != nil {
		return nil, fmt.Errorf("read name resolution policy: %w", err)
	}
	var rules []Rule
	for _, line := range strings.Split(out, "\n") {
		// Three fields, in the order the script above prints them. A rule we
		// cannot parse is a rule we would not have written, so it is reported
		// with an empty namespace and reconciled away.
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 || parts[0] == "" {
			continue
		}
		rules = append(rules, Rule{
			ID:        parts[0],
			Namespace: strings.ToLower(parts[1]),
			Servers:   splitList(parts[2]),
		})
	}
	return rules, nil
}

// mutate applies one plan in a single pass. Removals go first, so a namespace
// being repointed is never briefly covered by two rules at once.
func mutate(add, remove []string, server string) error {
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	stmts := make([]string, 0, len(add)+len(remove))
	for _, id := range remove {
		stmts = append(stmts, fmt.Sprintf(
			"Remove-DnsClientNrptRule -Name '%s' -Force -ErrorAction Stop", quote(id)))
	}
	for _, ns := range add {
		stmts = append(stmts, fmt.Sprintf(
			"Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s' -Comment '%s' -DisplayName '%s' -ErrorAction Stop | Out-Null",
			quote(ns), quote(server), Tag, quote(displayName(ns))))
	}
	if _, err := powershell(strings.Join(stmts, "; ")); err != nil {
		return fmt.Errorf("update name resolution policy: %w", err)
	}
	return nil
}

// displayName is what an administrator sees in Get-DnsClientNrptRule, so it
// says which product put the rule there and for which network.
func displayName(namespace string) string {
	return fmt.Sprintf("%s (%s)", shared.ProductName, strings.TrimPrefix(namespace, "."))
}

// powershell runs a script and returns its standard output.
//
// The DnsClient cmdlets are the supported way in: the registry behind the table
// is an undocumented layout, and the DNS client has to be told the table
// changed, which the cmdlets do and a registry write does not.
func powershell(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// quote escapes a value for a PowerShell single-quoted string.
func quote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
