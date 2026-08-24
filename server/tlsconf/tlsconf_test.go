package tlsconf

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"":            ModeAuto,
		"auto":        ModeAuto,
		"OFF":         ModeOff,
		"self":        ModeSelf,
		"self-signed": ModeSelf,
		" acme ":      ModeACME,
	} {
		got, err := ParseMode(in)
		if err != nil {
			t.Fatalf("ParseMode(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseMode("mtls"); err == nil {
		t.Fatal("ParseMode accepted an unknown mode")
	}
}

func TestBuildOffServesPlaintext(t *testing.T) {
	res, err := Build(Config{Mode: ModeOff})
	if err != nil {
		t.Fatal(err)
	}
	if res.TLS != nil {
		t.Fatal("ModeOff should not produce a TLS config")
	}
	if res.Scheme() != "http" {
		t.Fatalf("scheme = %q, want http", res.Scheme())
	}
	if res.Pin != "" {
		t.Fatal("ModeOff should not produce a pin")
	}
}

func TestAutoWithoutDomainsIsSelfSigned(t *testing.T) {
	dir := t.TempDir()
	res, err := Build(Config{Dir: dir, ListenAddr: "127.0.0.1:8787"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ModeSelf {
		t.Fatalf("mode = %q, want %q", res.Mode, ModeSelf)
	}
	if res.Pin == "" {
		t.Fatal("self-signed mode must publish a pin")
	}
	if res.Scheme() != "https" {
		t.Fatalf("scheme = %q, want https", res.Scheme())
	}
	if len(res.TLS.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(res.TLS.Certificates))
	}
	leaf, err := x509.ParseCertificate(res.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Fatalf("certificate does not cover localhost: %v", err)
	}
	if err := leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("certificate does not cover 127.0.0.1: %v", err)
	}
}

// The pin is handed to clients out of band, so it must survive both a restart
// and a certificate re-issue for new SANs.
func TestSelfSignedPinIsStableAcrossRestartAndReissue(t *testing.T) {
	dir := t.TempDir()
	first, err := Build(Config{Mode: ModeSelf, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Build(Config{Mode: ModeSelf, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if again.Pin != first.Pin {
		t.Fatalf("pin changed on restart: %q -> %q", first.Pin, again.Pin)
	}

	widened, err := Build(Config{Mode: ModeSelf, Dir: dir, ExtraSANs: []string{"vpn.example.test", "203.0.113.5"}})
	if err != nil {
		t.Fatal(err)
	}
	if widened.Pin != first.Pin {
		t.Fatalf("pin changed after re-issue: %q -> %q", first.Pin, widened.Pin)
	}
	leaf, err := x509.ParseCertificate(widened.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("vpn.example.test"); err != nil {
		t.Fatalf("re-issued certificate missing the new DNS SAN: %v", err)
	}
	if !containsIP(leaf.IPAddresses, net.ParseIP("203.0.113.5")) {
		t.Fatal("re-issued certificate missing the new IP SAN")
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); err != nil {
		t.Fatalf("private key not persisted: %v", err)
	}
}

func TestACMERequiresDomains(t *testing.T) {
	if _, err := Build(Config{Mode: ModeACME, Dir: t.TempDir()}); err == nil {
		t.Fatal("ACME without domains should fail")
	}
}

// TLS-ALPN-01 only works when the API itself answers on 443; anywhere else the
// server has to open an HTTP-01 listener instead.
func TestACMEAddsHTTPChallengeOffPort443(t *testing.T) {
	onDefault, err := Build(Config{Mode: ModeACME, Dir: t.TempDir(), Domains: []string{"vpn.example.test"}, ListenAddr: "0.0.0.0:443"})
	if err != nil {
		t.Fatal(err)
	}
	if onDefault.HTTPChallenge != nil {
		t.Fatal("no HTTP-01 listener is needed when serving :443")
	}
	offDefault, err := Build(Config{Mode: ModeACME, Dir: t.TempDir(), Domains: []string{"vpn.example.test"}, ListenAddr: "0.0.0.0:8443"})
	if err != nil {
		t.Fatal(err)
	}
	if offDefault.HTTPChallenge == nil {
		t.Fatal("serving off :443 needs an HTTP-01 listener")
	}
	if offDefault.Pin != "" {
		t.Fatal("ACME certificates are CA-verified and must not advertise a pin")
	}
}

func TestAutoWithDomainsPicksACME(t *testing.T) {
	res, err := Build(Config{Dir: t.TempDir(), Domains: []string{"vpn.example.test"}, ListenAddr: "0.0.0.0:443"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ModeACME {
		t.Fatalf("mode = %q, want %q", res.Mode, ModeACME)
	}
}
