//go:build windows

package nrpt

import (
	"net"
	"os"
	"strings"
	"testing"

	"github.com/Zeliper/zwan/client/resolver"
)

// TestListReadsTheRealTable runs the actual query against the actual table.
//
// It needs no privileges and changes nothing, and it is the only test that can
// catch a mistyped cmdlet, a quoting slip, or output this package can no longer
// parse — the kind of thing that otherwise only shows up on a user's machine.
func TestListReadsTheRealTable(t *testing.T) {
	rules, err := list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rules {
		// Only our own rules come back, and every one of them is a suffix rule.
		if r.ID == "" || !strings.HasPrefix(r.Namespace, ".") {
			t.Errorf("rule %+v does not look like one of ours", r)
		}
	}
	t.Logf("%d rule(s) of ours in the machine's policy table", len(rules))
}

// TestRealPolicyTable puts a rule in the machine's own policy table and lets
// Windows judge it: the name is looked up through the operating system's
// resolver rather than by asking ours directly, which is the entire point of
// the rule. Anything less only proves that a cmdlet ran.
//
// It needs Administrator and it changes machine state, so it runs only when
// asked for, and it puts the machine back either way:
//
//	go test ./client/nrpt/ -run RealPolicy -v     (elevated, ZWAN_NRPT_TEST=1)
//
// It also needs 127.0.0.1:53, so the engine service must not be running.
func TestRealPolicyTable(t *testing.T) {
	if os.Getenv("ZWAN_NRPT_TEST") == "" {
		t.Skip("set ZWAN_NRPT_TEST=1 in an elevated prompt to run this")
	}
	const suffix = "zwan-selftest"
	const want = "100.127.255.1"

	r := resolver.New()
	if _, err := r.Listen("127.0.0.1:53"); err != nil {
		t.Fatalf("resolver on 127.0.0.1:53 (is the engine service holding it?): %v", err)
	}
	go func() { _ = r.Serve() }()
	defer func() { _ = r.Shutdown() }()
	r.SetZone(suffix, map[string]net.IP{"host." + suffix: net.ParseIP(want)})

	// Whatever happens below, the machine gets its name resolution back.
	t.Cleanup(func() {
		if err := Purge(); err != nil {
			t.Errorf("purge: %v", err)
		}
		flushCache(t)
	})
	if err := Purge(); err != nil {
		t.Fatalf("purge before start: %v", err)
	}

	b, err := New("127.0.0.1:53")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Apply([]string{suffix}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	rules, err := list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 || rules[0].Namespace != "."+suffix {
		t.Fatalf("installed rules = %+v, want exactly one for .%s", rules, suffix)
	}

	// The system resolver, which is what every other program on the machine
	// uses. The cache is cleared first so the answer is the rule's, not an
	// older attempt's.
	flushCache(t)
	got, err := net.LookupHost("host." + suffix)
	if err != nil {
		t.Fatalf("system lookup of host.%s: %v", suffix, err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("host.%s resolved to %v, want [%s]", suffix, got, want)
	}

	// A rule covers its namespace and nothing else: ordinary name resolution
	// has to be exactly as it was.
	if _, err := net.LookupHost("localhost"); err != nil {
		t.Errorf("localhost stopped resolving: %v", err)
	}

	// Applying the same set again reconciles to the same single rule rather
	// than stacking a second one on top. Resync forces the comparison to be
	// made against the system instead of against what Apply remembers.
	b.Resync()
	if err := b.Apply([]string{suffix}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if rules, err := list(); err != nil || len(rules) != 1 {
		t.Fatalf("after re-apply: rules = %+v, err = %v; want exactly one", rules, err)
	}

	// A run that is killed rather than stopped leaves its rules behind, and the
	// next run has to find them by their mark and take them out. A binder that
	// has installed nothing and wants nothing is exactly that next run — and it
	// is the same path Clear takes.
	fresh, err := New("127.0.0.1:53")
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Apply(nil); err != nil {
		t.Fatalf("clear leftovers: %v", err)
	}
	if rules, err := list(); err != nil || len(rules) != 0 {
		t.Fatalf("after clearing leftovers: rules = %+v, err = %v; want none", rules, err)
	}
	flushCache(t)
	if addrs, err := net.LookupHost("host." + suffix); err == nil {
		t.Errorf("host.%s still resolved to %v after the rule was removed", suffix, addrs)
	}
}

// flushCache empties the DNS client cache, so a lookup measures the rule rather
// than the answer to an earlier one.
func flushCache(t *testing.T) {
	t.Helper()
	if _, err := powershell("Clear-DnsClientCache"); err != nil {
		t.Logf("clear dns cache: %v", err)
	}
}
