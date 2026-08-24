// Package acl implements the access policy (design docs 18 and 40): which
// groups of members may reach each other, and which groups may use a published
// service.
//
// It is shared rather than server-side because both ends enforce a piece of it.
// The control server decides who appears in whose peer and service lists; the
// node hosting a service also checks the source of each connection, so knowing
// an address and a port is not enough to reach a service you are not allowed to
// use.
//
// A member's group comes from the join token it registered with, so an admin
// hands out a different token per group. Rules are written between group names,
// with "*" matching any group.
//
// An empty policy allows everything — a single-group home network needs no
// rules. As soon as one rule exists the policy is default-deny, so adding a rule
// never silently widens access.
package acl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	// Wildcard matches any group on either side of a rule.
	Wildcard = "*"
	// DefaultGroup is the group of members that joined with the plain join token.
	DefaultGroup = "default"
)

// Rule allows every group in Src to reach every group in Dst.
type Rule struct {
	Src []string `json:"src"`
	Dst []string `json:"dst"`
}

// Policy is an ordered set of allow rules.
type Policy struct {
	Rules []Rule `json:"rules"`
}

// Empty reports whether the policy has no rules, meaning "allow everything".
func (p *Policy) Empty() bool { return p == nil || len(p.Rules) == 0 }

// Allows reports whether members of group src may reach members of group dst.
func (p *Policy) Allows(src, dst string) bool {
	if p.Empty() {
		return true
	}
	src, dst = normalize(src), normalize(dst)
	for _, r := range p.Rules {
		if matches(r.Src, src) && matches(r.Dst, dst) {
			return true
		}
	}
	return false
}

// Connected reports whether two members may share a tunnel at all.
//
// This is deliberately symmetric: a WireGuard tunnel needs both ends to hold the
// other's key, so peer visibility cannot be one-way. Direction only becomes
// meaningful for services, which have a clear destination — see AllowsGroup.
func (p *Policy) Connected(a, b string) bool {
	return p.Allows(a, b) || p.Allows(b, a)
}

// AllowsGroup reports whether group is covered by a service's allow list. An
// empty list means the service is open to everyone who can reach its node.
func AllowsGroup(list []string, group string) bool {
	if len(list) == 0 {
		return true
	}
	return matches(list, normalize(group))
}

// Parse reads a JSON policy document:
//
//	{"rules": [{"src": ["dev"], "dst": ["*"]}]}
func Parse(data []byte) (*Policy, error) {
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse ACL policy: %w", err)
	}
	for i, r := range p.Rules {
		if len(r.Src) == 0 || len(r.Dst) == 0 {
			return nil, fmt.Errorf("parse ACL policy: rule %d needs both src and dst", i+1)
		}
	}
	return &p, nil
}

// ParseRule reads the shorthand used on the command line: "dev->*", or
// "guest,friends -> nas" for several groups on a side.
func ParseRule(s string) (Rule, error) {
	src, dst, ok := strings.Cut(s, "->")
	if !ok {
		return Rule{}, fmt.Errorf("ACL rule %q: expected \"<src groups>-><dst groups>\"", s)
	}
	r := Rule{Src: splitGroups(src), Dst: splitGroups(dst)}
	if len(r.Src) == 0 || len(r.Dst) == 0 {
		return Rule{}, fmt.Errorf("ACL rule %q: both sides need at least one group", s)
	}
	return r, nil
}

// String renders a rule back into the shorthand form, for logs.
func (r Rule) String() string {
	return strings.Join(r.Src, ",") + "->" + strings.Join(r.Dst, ",")
}

func splitGroups(s string) []string {
	var out []string
	for _, g := range strings.Split(s, ",") {
		if g = normalize(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

func matches(list []string, group string) bool {
	for _, g := range list {
		if g == Wildcard || normalize(g) == group {
			return true
		}
	}
	return false
}

// normalize makes group names case-insensitive and forgiving of stray spaces,
// so a policy file and a --join-token flag cannot disagree over "Dev" vs "dev".
func normalize(g string) string { return strings.ToLower(strings.TrimSpace(g)) }

// JoinTokens maps a join token to the group its holder lands in. An admin hands
// a different token to each group, which is how a member acquires one: nothing
// the client sends decides its own group.
type JoinTokens map[string]string

// Group returns the group a join token admits to.
func (t JoinTokens) Group(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	g, ok := t[token]
	if !ok {
		return "", false
	}
	if g == "" {
		g = DefaultGroup
	}
	return normalize(g), true
}

// Add registers a token for a group, defaulting a blank group to DefaultGroup.
func (t JoinTokens) Add(group, token string) {
	if token == "" {
		return
	}
	if normalize(group) == "" {
		group = DefaultGroup
	}
	t[token] = normalize(group)
}

// Groups lists the configured group names, sorted, for display.
func (t JoinTokens) Groups() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range t {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

// ParseJoinToken reads the "<group>=<token>" form used on the command line.
func ParseJoinToken(s string) (group, token string, err error) {
	group, token, ok := strings.Cut(s, "=")
	if !ok || normalize(group) == "" || strings.TrimSpace(token) == "" {
		return "", "", fmt.Errorf("join token %q: expected \"<group>=<token>\"", s)
	}
	return normalize(group), strings.TrimSpace(token), nil
}

// BuildJoinTokens assembles the token table from the default-group token and a
// group -> token map.
//
// A token shared by two groups is rejected rather than resolved: the group is
// what grants access, so an ambiguous token is a hole, and silently letting one
// group win would be the worst outcome.
func BuildJoinTokens(defaultToken string, groupTokens map[string]string) (JoinTokens, error) {
	out := JoinTokens{}
	owner := map[string]string{}
	add := func(group, token string) error {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil
		}
		if normalize(group) == "" {
			group = DefaultGroup
		}
		if prev, ok := owner[token]; ok && prev != normalize(group) {
			return fmt.Errorf("join token is shared by groups %q and %q; give each group its own token", prev, normalize(group))
		}
		owner[token] = normalize(group)
		out.Add(group, token)
		return nil
	}
	if err := add(DefaultGroup, defaultToken); err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(groupTokens))
	for g := range groupTokens {
		groups = append(groups, g)
	}
	sort.Strings(groups) // deterministic error messages
	for _, g := range groups {
		if err := add(g, groupTokens[g]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
