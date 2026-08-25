package main

import (
	"strings"
	"sync"
	"testing"
)

// reset puts the package-level language back, so tests do not leak into each
// other through it.
func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		langMu.Lock()
		current = langEN
		onChange = nil
		langMu.Unlock()
	})
}

func TestSetLanguageSwitchesTheTrayWording(t *testing.T) {
	reset(t)
	app := &App{}

	app.SetLanguage("ko-KR")
	if got := text().quit; got == trayStrings[langEN].quit {
		t.Fatalf("Quit is still %q after switching to Korean", got)
	}
	app.SetLanguage("en-US")
	if got := text().quit; got != trayStrings[langEN].quit {
		t.Fatalf("Quit is %q after switching back to English, want %q", got, trayStrings[langEN].quit)
	}
}

// The window reports a BCP-47 tag, which carries a region the tray does not care
// about. Matching on the language subtag keeps "ko", "ko-KR" and "ko-Kore-KR"
// all meaning the same thing.
func TestSetLanguageIgnoresTheRegion(t *testing.T) {
	reset(t)
	app := &App{}
	for _, tag := range []string{"ko", "ko-KR", "KO-kr", "ko-Kore-KR"} {
		app.SetLanguage("en")
		app.SetLanguage(tag)
		if text().quit != trayStrings[langKO].quit {
			t.Errorf("%q did not select Korean", tag)
		}
	}
	for _, tag := range []string{"en", "en-GB", "fr-FR", "", "nonsense"} {
		app.SetLanguage("ko")
		app.SetLanguage(tag)
		if text().quit != trayStrings[langEN].quit {
			t.Errorf("%q did not fall back to English", tag)
		}
	}
}

// systray cannot rebuild its menu, so items titled once at startup have to be
// retitled on a change. Without this hook the menu keeps whatever language it
// was built in, which is the language of the operating system rather than the
// one the user picked.
func TestLanguageChangeNotifiesTheMenu(t *testing.T) {
	reset(t)
	app := &App{}

	var mu sync.Mutex
	calls := 0
	onLanguageChange(func() {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	app.SetLanguage("ko")
	app.SetLanguage("ko") // already Korean: nothing changed, so nothing to redraw
	app.SetLanguage("en")

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("hook ran %d times, want 2 (once per actual change)", calls)
	}
}

func TestEveryTrayStringIsTranslated(t *testing.T) {
	en, ko := trayStrings[langEN], trayStrings[langKO]
	if en == (trayText{}) {
		t.Fatal("the English tray strings are empty")
	}
	// Compare field by field: a Korean entry left equal to the English one is a
	// string someone forgot, and an empty one is a menu item with no label.
	cases := []struct {
		name   string
		en, ko string
	}{
		{"show", en.show, ko.show},
		{"showTip", en.showTip, ko.showTip},
		{"stopHosting", en.stopHosting, ko.stopHosting},
		{"stopHostTip", en.stopHostTip, ko.stopHostTip},
		{"quit", en.quit, ko.quit},
		{"quitTip", en.quitTip, ko.quitTip},
		{"tooltip", en.tooltip, ko.tooltip},
		{"idle", en.idle, ko.idle},
		{"joinedPrefix", en.joinedPrefix, ko.joinedPrefix},
		{"hostingPrefix", en.hostingPrefix, ko.hostingPrefix},
		{"disconnected", en.disconnected, ko.disconnected},
		{"connected", en.connected, ko.connected},
		{"leave", en.leave, ko.leave},
		{"join", en.join, ko.join},
	}
	for _, c := range cases {
		if strings.TrimSpace(c.ko) == "" {
			t.Errorf("%s has no Korean text", c.name)
		}
		if c.ko == c.en {
			t.Errorf("%s is identical in both languages (%q) — untranslated?", c.name, c.en)
		}
	}
}

func TestTooltipFollowsTheLanguage(t *testing.T) {
	reset(t)
	app := &App{}

	app.SetLanguage("en")
	if got := tooltip(nil, ""); got != trayStrings[langEN].idle {
		t.Errorf("idle tooltip = %q, want %q", got, trayStrings[langEN].idle)
	}
	if got := tooltip([]string{"alice"}, ""); !strings.Contains(got, "alice") {
		t.Errorf("tooltip %q does not name the network", got)
	}

	app.SetLanguage("ko")
	if got := tooltip(nil, ""); got != trayStrings[langKO].idle {
		t.Errorf("idle tooltip = %q, want the Korean one", got)
	}
	// The alias is the operator's own word and stays as they typed it, in any
	// language.
	if got := tooltip([]string{"alice"}, ""); !strings.Contains(got, "alice") {
		t.Errorf("tooltip %q dropped the network name", got)
	}
}
