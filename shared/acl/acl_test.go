package acl

import (
	"reflect"
	"testing"
)

// A network with no rules is a network where everyone talks to everyone, which
// is what a single-group home setup wants.
func TestEmptyPolicyAllowsEverything(t *testing.T) {
	var p *Policy
	if !p.Empty() || !p.Allows("dev", "guest") || !p.Connected("a", "b") {
		t.Fatal("a nil policy must allow everything")
	}
	empty := &Policy{}
	if !empty.Empty() || !empty.Allows("dev", "guest") {
		t.Fatal("a policy with no rules must allow everything")
	}
}

// The moment a rule exists the policy is default-deny, so adding one never
// silently widens access.
func TestOneRuleMakesEverythingElseDenied(t *testing.T) {
	p := &Policy{Rules: []Rule{{Src: []string{"dev"}, Dst: []string{"nas"}}}}
	if !p.Allows("dev", "nas") {
		t.Fatal("the configured rule should allow its own pair")
	}
	for _, c := range [][2]string{{"guest", "nas"}, {"dev", "guest"}, {"nas", "dev"}} {
		if p.Allows(c[0], c[1]) {
			t.Fatalf("%s->%s should be denied by default", c[0], c[1])
		}
	}
}

func TestWildcardAndCaseInsensitivity(t *testing.T) {
	p := &Policy{Rules: []Rule{{Src: []string{"Dev"}, Dst: []string{Wildcard}}}}
	if !p.Allows("dev", "anything") || !p.Allows("  DEV  ", "guest") {
		t.Fatal("group names should match case- and space-insensitively")
	}
	if p.Allows("guest", "dev") {
		t.Fatal("the wildcard is on the destination side only")
	}
}

// Peer visibility cannot be one-way: a WireGuard tunnel needs both ends to hold
// the other's key, so Connected has to consider both directions.
func TestConnectedIsSymmetricEvenThoughAllowsIsNot(t *testing.T) {
	p := &Policy{Rules: []Rule{{Src: []string{"guest"}, Dst: []string{"nas"}}}}
	if p.Allows("nas", "guest") {
		t.Fatal("Allows must stay directional")
	}
	if !p.Connected("nas", "guest") || !p.Connected("guest", "nas") {
		t.Fatal("Connected must hold in both directions")
	}
	if p.Connected("guest", "dev") {
		t.Fatal("Connected must not invent pairs no rule covers")
	}
}

func TestAllowsGroup(t *testing.T) {
	if !AllowsGroup(nil, "guest") || !AllowsGroup([]string{}, "guest") {
		t.Fatal("a service with no allow list is open to everyone who can reach its node")
	}
	if !AllowsGroup([]string{"dev", "ops"}, "OPS") {
		t.Fatal("allow lists should match case-insensitively")
	}
	if AllowsGroup([]string{"dev"}, "guest") {
		t.Fatal("a group outside the allow list must be refused")
	}
	if !AllowsGroup([]string{Wildcard}, "anyone") {
		t.Fatal("the wildcard should open a service to every group")
	}
}

func TestParseRule(t *testing.T) {
	r, err := ParseRule(" Dev , ops -> * ")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Src, []string{"dev", "ops"}) || !reflect.DeepEqual(r.Dst, []string{"*"}) {
		t.Fatalf("rule = %+v", r)
	}
	if got := r.String(); got != "dev,ops->*" {
		t.Fatalf("String() = %q", got)
	}
	for _, bad := range []string{"dev", "->nas", "dev->", "", " -> "} {
		if _, err := ParseRule(bad); err == nil {
			t.Fatalf("ParseRule(%q) accepted a malformed rule", bad)
		}
	}
}

func TestParsePolicyDocument(t *testing.T) {
	p, err := Parse([]byte(`{"rules":[{"src":["dev"],"dst":["*"]},{"src":["guest"],"dst":["nas"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 2 || !p.Allows("dev", "nas") || !p.Allows("guest", "nas") || p.Allows("guest", "dev") {
		t.Fatalf("policy = %+v", p.Rules)
	}
	for _, bad := range []string{`{`, `{"rules":[{"src":["dev"]}]}`, `{"rules":[{"dst":["dev"]}]}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatalf("Parse(%q) accepted an invalid document", bad)
		}
	}
}

// A member's group comes from the token it joined with, never from anything it
// sends.
func TestJoinTokens(t *testing.T) {
	tokens := JoinTokens{}
	tokens.Add("", "plain-token")
	tokens.Add("Dev", "dev-token")
	tokens.Add("guest", "")

	if g, ok := tokens.Group("plain-token"); !ok || g != DefaultGroup {
		t.Fatalf("blank group = (%q, %v), want the default group", g, ok)
	}
	if g, ok := tokens.Group("dev-token"); !ok || g != "dev" {
		t.Fatalf("dev token = (%q, %v)", g, ok)
	}
	if _, ok := tokens.Group("unknown"); ok {
		t.Fatal("an unknown token must not admit anyone")
	}
	if _, ok := tokens.Group(""); ok {
		t.Fatal("an empty token must not admit anyone")
	}
	if got := tokens.Groups(); !reflect.DeepEqual(got, []string{DefaultGroup, "dev"}) {
		t.Fatalf("Groups() = %v", got)
	}
}

func TestParseJoinToken(t *testing.T) {
	g, tok, err := ParseJoinToken("Dev=abc123")
	if err != nil || g != "dev" || tok != "abc123" {
		t.Fatalf("= (%q, %q, %v)", g, tok, err)
	}
	for _, bad := range []string{"abc123", "=abc", "dev=", ""} {
		if _, _, err := ParseJoinToken(bad); err == nil {
			t.Fatalf("ParseJoinToken(%q) accepted a malformed value", bad)
		}
	}
}
