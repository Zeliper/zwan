package nrpt

import (
	"strings"
	"testing"
)

func TestNewRejectsAnythingButPort53(t *testing.T) {
	if _, err := New("127.0.0.1:53"); err != nil {
		t.Fatalf("127.0.0.1:53: %v", err)
	}
	// A rule names its server by address alone, so a resolver anywhere else is
	// unreachable through the policy table and has to be refused up front.
	for _, addr := range []string{"127.0.0.1:5353", "127.0.0.1", "", "not-an-address:53"} {
		if _, err := New(addr); err == nil {
			t.Errorf("New(%q) was accepted; it should not be", addr)
		}
	}
}

func TestNamespacesAreSuffixRulesWithoutDuplicates(t *testing.T) {
	got := namespaces([]string{"Bob", "alice", "alice.", " ALICE ", "", "."})
	want := []string{".alice", ".bob"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("namespaces = %v, want %v", got, want)
	}
}

func TestNamespacesDropAnythingThatIsNotAPlainName(t *testing.T) {
	// These reach the operating system as a command line, so a suffix that a
	// control server made up must not be able to carry anything else in.
	for _, bad := range []string{"a'; Remove-Item -Recurse #", "al ice", "a..b", "aliceが", "a/b"} {
		if got := namespaces([]string{bad}); len(got) != 0 {
			t.Errorf("namespaces(%q) = %v, want none", bad, got)
		}
	}
}

func TestPlanAddsWhatIsMissingAndRemovesWhatIsNotWanted(t *testing.T) {
	have := []Rule{
		{ID: "{keep}", Namespace: ".alice", Servers: []string{"127.0.0.1"}},
		{ID: "{stale}", Namespace: ".gone", Servers: []string{"127.0.0.1"}},
	}
	add, remove := plan([]string{".alice", ".bob"}, have, "127.0.0.1")
	if len(add) != 1 || add[0] != ".bob" {
		t.Errorf("add = %v, want [.bob]", add)
	}
	if len(remove) != 1 || remove[0] != "{stale}" {
		t.Errorf("remove = %v, want [{stale}]", remove)
	}
}

func TestPlanRewritesARuleThatPointsElsewhere(t *testing.T) {
	have := []Rule{{ID: "{wrong}", Namespace: ".alice", Servers: []string{"10.0.0.1"}}}
	add, remove := plan([]string{".alice"}, have, "127.0.0.1")
	if len(remove) != 1 || remove[0] != "{wrong}" {
		t.Errorf("remove = %v, want [{wrong}]", remove)
	}
	if len(add) != 1 || add[0] != ".alice" {
		t.Errorf("add = %v, want [.alice]", add)
	}
}

func TestPlanKeepsOneRulePerNamespace(t *testing.T) {
	have := []Rule{
		{ID: "{first}", Namespace: ".alice", Servers: []string{"127.0.0.1"}},
		{ID: "{second}", Namespace: ".alice", Servers: []string{"127.0.0.1"}},
	}
	add, remove := plan([]string{".alice"}, have, "127.0.0.1")
	if len(add) != 0 {
		t.Errorf("add = %v, want none", add)
	}
	if len(remove) != 1 || remove[0] != "{second}" {
		t.Errorf("remove = %v, want [{second}]", remove)
	}
}

// A crash leaves rules behind, and the next run has to take them out: nothing
// is listening on the resolver's address until the engines come back up, so a
// leftover rule is a suffix that fails instead of falling through.
func TestPlanWithNothingWantedRemovesEverythingOfOurs(t *testing.T) {
	have := []Rule{
		{ID: "{a}", Namespace: ".alice", Servers: []string{"127.0.0.1"}},
		{ID: "{b}", Namespace: ".bob", Servers: []string{"127.0.0.1"}},
	}
	add, remove := plan(nil, have, "127.0.0.1")
	if len(add) != 0 {
		t.Errorf("add = %v, want none", add)
	}
	if len(remove) != 2 {
		t.Errorf("remove = %v, want both", remove)
	}
}

// A namespace read back from the system may be fully qualified with a trailing
// dot. Comparing it raw would make a rule fail to match itself, and the binder
// would replace it on every reconcile forever.
func TestPlanMatchesANamespaceWrittenAsAnFQDN(t *testing.T) {
	have := []Rule{{ID: "{keep}", Namespace: ".Alice.", Servers: []string{"127.0.0.1"}}}
	add, remove := plan([]string{".alice"}, have, "127.0.0.1")
	if len(add) != 0 || len(remove) != 0 {
		t.Fatalf("add = %v, remove = %v; want the rule left alone", add, remove)
	}
}
