package firewall

import (
	"strings"
	"testing"
)

func find(args []string, prefix string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix), true
		}
	}
	return "", false
}

func TestAddNamesTheProgramAndNotAPort(t *testing.T) {
	args := addArgs(Rule{Name: "zwan control server", Exe: `C:\Program Files\zwan\zwan-server.exe`, Description: "why"})

	if got, ok := find(args, "program="); !ok || got != `C:\Program Files\zwan\zwan-server.exe` {
		t.Errorf("program = %q (present: %v)", got, ok)
	}
	if got, _ := find(args, "name="); got != "zwan control server" {
		t.Errorf("name = %q", got)
	}
	if got, _ := find(args, "dir="); got != "in" {
		t.Errorf("dir = %q, want in", got)
	}
	if got, _ := find(args, "action="); got != "allow" {
		t.Errorf("action = %q, want allow", got)
	}
	// A port would be the wrong thing to pin a rule to: the control, relay and
	// tunnel ports are all configurable, and a rule for the wrong port is as
	// silent as no rule at all.
	if _, ok := find(args, "localport="); ok {
		t.Error("the rule names a port; it should name the program")
	}
	if _, ok := find(args, "protocol="); ok {
		t.Error("the rule names a protocol; the program listens on both TCP and UDP")
	}
}

// A machine hosting a network is reached from the internet, and Windows usually
// files that connection under Public. A rule that skipped Public would be a rule
// that does not apply where it is needed.
func TestAddCoversEveryNetworkProfile(t *testing.T) {
	args := addArgs(Rule{Name: "n", Exe: "e"})
	if got, _ := find(args, "profile="); got != "any" {
		t.Errorf("profile = %q, want any", got)
	}
	if got, _ := find(args, "enable="); got != "yes" {
		t.Errorf("enable = %q, want yes", got)
	}
}

func TestDeleteMatchesByName(t *testing.T) {
	args := deleteArgs("zwan control server")
	if got, _ := find(args, "name="); got != "zwan control server" {
		t.Errorf("name = %q", got)
	}
	if args[2] != "delete" || args[3] != "rule" {
		t.Errorf("args = %v, want a rule deletion", args)
	}
}

// Removing a rule that was never there is the ordinary first install, not a
// failure. Windows says so in the user's own language, so the check has to
// recognise more than the English.
func TestNotFoundRecognisesBothLanguages(t *testing.T) {
	for _, out := range []string{
		"No rules match the specified criteria.",
		"지정한 조건과 일치하는 규칙이 없습니다.",
	} {
		if !notFound(out) {
			t.Errorf("notFound(%q) = false, want true", out)
		}
	}
	for _, out := range []string{
		"The requested operation requires elevation (Run as administrator).",
		"요청한 작업에는 권한 상승이 필요합니다.",
		"",
	} {
		if notFound(out) {
			t.Errorf("notFound(%q) = true; a real failure must not be swallowed", out)
		}
	}
}
