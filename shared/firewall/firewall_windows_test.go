//go:build windows

package firewall

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRealRuleRoundTrip puts a rule in the machine's actual firewall and reads
// it back, which is the only way to know the arguments mean what they are meant
// to mean. Nothing below proves anything if netsh merely accepted the words.
//
// It changes machine state and needs Administrator, so it runs only when asked
// for, and it takes the rule out again either way:
//
//	go test ./shared/firewall/ -run RealRule -v     (elevated, ZWAN_FIREWALL_TEST=1)
func TestRealRuleRoundTrip(t *testing.T) {
	if os.Getenv("ZWAN_FIREWALL_TEST") == "" {
		t.Skip("set ZWAN_FIREWALL_TEST=1 in an elevated prompt to run this")
	}
	rule := Rule{
		Name:        "zwan self-test (safe to delete)",
		Exe:         `C:\Windows\System32\zwan-does-not-exist.exe`,
		Description: "Created by a zwan test; it should not outlive it.",
	}
	t.Cleanup(func() {
		if err := Remove(rule.Name); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if show(t, rule.Name) != "" {
			t.Errorf("the rule survived removal")
		}
	})

	if err := Allow(rule); err != nil {
		t.Fatalf("allow: %v", err)
	}
	out := show(t, rule.Name)
	if out == "" {
		t.Fatal("the rule was not installed")
	}
	// Windows localises the field labels, so the values are what gets checked.
	if !strings.Contains(strings.ToLower(out), strings.ToLower(rule.Exe)) {
		t.Errorf("the rule does not name the program:\n%s", out)
	}

	// Installing again must leave one rule, not two: netsh adds rather than
	// replaces, and a pile of duplicates is what an upgrade would produce.
	if err := Allow(rule); err != nil {
		t.Fatalf("allow again: %v", err)
	}
	if n := strings.Count(show(t, rule.Name), rule.Exe); n != 1 {
		t.Errorf("the program appears %d times after a second install, want 1", n)
	}

	// Removing something that is not there is the ordinary first install.
	if err := Remove("zwan rule that was never created"); err != nil {
		t.Errorf("removing an absent rule reported an error: %v", err)
	}
}

func show(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("netsh", "advfirewall", "firewall", "show", "rule",
		"name="+name, "verbose").CombinedOutput()
	if err != nil {
		return "" // "no rules match" exits non-zero
	}
	return string(out)
}
