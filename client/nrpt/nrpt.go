// Package nrpt binds this device's local resolver into Windows' own name
// resolution, so a joined network's names work in every program on the machine
// and not only for whoever queries the resolver directly.
//
// Windows' Name Resolution Policy Table sends a namespace to a chosen DNS
// server ahead of whatever the interfaces would otherwise use. One rule per
// joined network points "*.<alias>" at the resolver and leaves every other name
// — the whole internet — on the system's usual path (design doc 39).
//
// A rule outlives the process that wrote it: it is machine state, not a handle.
// So every rule this package installs carries a comment naming it as ours, and
// the binder reconciles against what is actually installed rather than against
// what it believes it installed. A crash leaves rules behind, and the next
// Apply — or Purge — finds them by that mark and takes them out.
package nrpt

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
)

// Tag marks every rule this program installs. The table is machine-wide and may
// hold rules from a domain policy or another program, so telling ours apart has
// to work from the rule itself, after any number of restarts.
const Tag = "zwan-split-dns"

// Rule is one entry of the system's policy table.
type Rule struct {
	ID        string   // the system's own identifier for the rule
	Namespace string   // what the rule matches, e.g. ".alice"
	Servers   []string // where it sends those names
}

// Binder keeps the system's rules equal to a set of DNS suffixes.
type Binder struct {
	server string // resolver address, without a port

	mu     sync.Mutex
	synced bool
	last   string
}

// New returns a binder pointing the system at a resolver listening on addr.
//
// A policy rule names its server by address alone, so the resolver has to own
// port 53: there is nowhere to put a port number. Any other port is refused
// here rather than turned into rules that send names somewhere nothing answers.
func New(addr string) (*Binder, error) {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return nil, fmt.Errorf("resolver address %q: %w", addr, err)
	}
	if ap.Port() != 53 {
		return nil, fmt.Errorf("system DNS needs the resolver on port 53, not %d", ap.Port())
	}
	return &Binder{server: ap.Addr().String()}, nil
}

// Apply makes the system's rules match suffixes exactly: a rule is added for
// every suffix that has none, and any of our rules for anything else is
// removed. Rules belonging to something else are never touched.
//
// It is idempotent and cheap to call often: an unchanged set is recognised
// without going near the operating system.
func (b *Binder) Apply(suffixes []string) error {
	want := namespaces(suffixes)
	sig := strings.Join(want, ",")

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.synced && sig == b.last {
		return nil
	}

	have, err := list()
	if err != nil {
		return err
	}
	add, remove := plan(want, have, b.server)
	if err := mutate(add, remove, b.server); err != nil {
		return err
	}
	b.synced, b.last = true, sig
	return nil
}

// Resync forgets what the last Apply installed, so the next one reads the
// system's table again instead of trusting its own memory of it. Rules are
// machine state, and a policy refresh or an administrator can change them
// underneath us.
func (b *Binder) Resync() {
	b.mu.Lock()
	b.synced = false
	b.mu.Unlock()
}

// Clear removes every rule this binder installed. Leaving them behind would
// send every name under a joined suffix to a resolver that stopped listening.
func (b *Binder) Clear() error { return b.Apply(nil) }

// Purge removes every rule this program installed, whichever run installed
// them. Uninstalling has to leave the machine's name resolution as it found it.
func Purge() error { return (&Binder{}).Apply(nil) }

// plan works out the difference between the rules installed and the namespaces
// wanted. have holds only our own rules, so everything in it is ours to remove.
//
// A rule for a wanted namespace that points anywhere but at our resolver is
// removed and added back rather than edited: the set is tiny, and a rule that
// is not exactly what we would write is not one to keep.
func plan(want []string, have []Rule, server string) (add, remove []string) {
	wanted := make(map[string]bool, len(want))
	for _, ns := range want {
		wanted[ns] = true
	}
	found := make(map[string]bool, len(want))
	for _, r := range have {
		ns := sameNamespace(r.Namespace)
		// A duplicate for a namespace we already have is dropped too: two rules
		// for one name is the system picking one of them for us.
		if !wanted[ns] || found[ns] || !servedBy(r, server) {
			remove = append(remove, r.ID)
			continue
		}
		found[ns] = true
	}
	for _, ns := range want {
		if !found[ns] {
			add = append(add, ns)
		}
	}
	return add, remove
}

// sameNamespace puts a namespace read back from the system into the form
// namespaces produces, so the two can be compared.
//
// The trailing dot is trimmed because a fully qualified name may carry one and
// we never write one: without this, a rule would be found not to match itself
// and be replaced on every reconcile.
func sameNamespace(ns string) string {
	ns = strings.ToLower(strings.TrimSpace(ns))
	if len(ns) > 1 {
		ns = strings.TrimSuffix(ns, ".")
	}
	return ns
}

// servedBy reports whether a rule sends its namespace to our resolver and
// nowhere else.
func servedBy(r Rule, server string) bool {
	return len(r.Servers) == 1 && strings.EqualFold(strings.TrimSpace(r.Servers[0]), server)
}

// namespaces turns suffixes into the form the policy table matches on, sorted
// and without duplicates. The leading dot is what makes a rule cover everything
// under a name rather than the one name itself.
//
// Anything that is not a plain DNS name is dropped: a suffix reaches here from
// a control server as well as from the operator, and it ends up in a command
// line either way.
func namespaces(suffixes []string) []string {
	seen := make(map[string]bool, len(suffixes))
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		s = strings.ToLower(strings.Trim(strings.TrimSpace(s), "."))
		if s == "" || seen[s] || !plainName(s) {
			continue
		}
		seen[s] = true
		out = append(out, "."+s)
	}
	sort.Strings(out)
	return out
}

// plainName reports whether s is made only of the characters a DNS suffix is
// allowed to use.
func plainName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return false
		}
	}
	return !strings.Contains(s, "..")
}
