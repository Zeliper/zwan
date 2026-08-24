package certpin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testSPKI(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spki, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	return spki
}

func TestOfSPKIIsStableAndPrefixed(t *testing.T) {
	spki := testSPKI(t)
	got := OfSPKI(spki)
	if !strings.HasPrefix(got, Prefix) {
		t.Fatalf("pin %q missing %q prefix", got, Prefix)
	}
	if again := OfSPKI(spki); again != got {
		t.Fatalf("pin not stable: %q vs %q", got, again)
	}
	if other := OfSPKI(testSPKI(t)); other == got {
		t.Fatal("different keys produced the same pin")
	}
}

func TestNormalizeAcceptsHexAndBase64(t *testing.T) {
	canonical := OfSPKI(testSPKI(t))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(canonical, Prefix))
	if err != nil {
		t.Fatal(err)
	}
	forms := []string{
		canonical,
		strings.TrimPrefix(canonical, Prefix),
		"SHA256:" + hex.EncodeToString(raw),
		hex.EncodeToString(raw),
	}
	for _, f := range forms {
		got, err := Normalize(f)
		if err != nil {
			t.Fatalf("Normalize(%q): %v", f, err)
		}
		if got != canonical {
			t.Fatalf("Normalize(%q) = %q, want %q", f, got, canonical)
		}
	}
}

func TestNormalizeRejectsBadInput(t *testing.T) {
	for _, bad := range []string{"", "   ", "sha1:abcd", "sha256:not-base64!!", "sha256:" + base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := Normalize(bad); err == nil {
			t.Fatalf("Normalize(%q) accepted an invalid pin", bad)
		}
	}
}

func TestVerifierMatchesOnlyThePinnedKey(t *testing.T) {
	certDER, spki := selfSignedForTest(t)
	verify, err := Verifier(OfSPKI(spki))
	if err != nil {
		t.Fatal(err)
	}
	if err := verify([][]byte{certDER}, nil); err != nil {
		t.Fatalf("pinned certificate rejected: %v", err)
	}

	otherDER, _ := selfSignedForTest(t)
	if err := verify([][]byte{otherDER}, nil); err == nil {
		t.Fatal("verifier accepted a certificate with a different key")
	}
	if err := verify(nil, nil); err == nil {
		t.Fatal("verifier accepted an empty chain")
	}
}

func TestHTTPClientWithoutPinUsesSystemTrust(t *testing.T) {
	c, err := HTTPClient("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport != nil {
		t.Fatal("unpinned client should keep the default transport")
	}
	if _, err := HTTPClient("nonsense", 0); err == nil {
		t.Fatal("HTTPClient accepted an invalid pin")
	}
}

// selfSignedForTest returns a fresh self-signed certificate and its SPKI DER.
func selfSignedForTest(t *testing.T) (certDER, spki []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	spki, err = x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	return der, spki
}
