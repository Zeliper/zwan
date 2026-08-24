package keys

import "testing"

func TestGenerateRoundTrip(t *testing.T) {
	p, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	priv2, err := ParsePrivate(p.String())
	if err != nil {
		t.Fatalf("parse private: %v", err)
	}
	if priv2.String() != p.String() {
		t.Fatal("private key round-trip mismatch")
	}
	pub2, err := ParsePublic(p.Public().String())
	if err != nil {
		t.Fatalf("parse public: %v", err)
	}
	if pub2.String() != p.Public().String() {
		t.Fatal("public key round-trip mismatch")
	}
}

func TestGenerateDistinct(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if a.String() == b.String() {
		t.Fatal("two generated keys are identical")
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := ParsePublic("not-base64!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
}
