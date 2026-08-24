// Package certpin implements SPKI certificate pinning — the trust anchor used
// when a control server has no public domain (design doc 36 / 47.1). Instead of
// a CA chain, the server publishes the SHA-256 fingerprint of its public key and
// the client verifies that exact fingerprint.
//
// The fingerprint covers the SubjectPublicKeyInfo, not the certificate, so the
// server can re-issue its certificate (new SANs, later expiry) with the same key
// without invalidating pins that were already handed out.
package certpin

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Prefix marks a canonical fingerprint string.
const Prefix = "sha256:"

// OfSPKI returns the canonical pin for a DER-encoded SubjectPublicKeyInfo.
func OfSPKI(spki []byte) string {
	sum := sha256.Sum256(spki)
	return Prefix + base64.StdEncoding.EncodeToString(sum[:])
}

// OfCert returns the canonical pin for a parsed certificate.
func OfCert(c *x509.Certificate) string { return OfSPKI(c.RawSubjectPublicKeyInfo) }

// Normalize accepts a pin written as base64 or hex, with or without the
// "sha256:" prefix, and returns the canonical form. Empty input is an error;
// callers that treat "no pin" as "use the system trust store" must check first.
func Normalize(pin string) (string, error) {
	s := strings.TrimSpace(pin)
	if s == "" {
		return "", errors.New("empty pin")
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		algo := strings.ToLower(s[:i])
		if algo != "sha256" {
			return "", fmt.Errorf("unsupported pin algorithm %q (want sha256)", s[:i])
		}
		s = s[i+1:]
	}
	var raw []byte
	switch {
	case len(s) == sha256.Size*2:
		b, err := hex.DecodeString(s)
		if err != nil {
			return "", fmt.Errorf("pin is not valid hex: %w", err)
		}
		raw = b
	default:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			// tolerate URL-safe / unpadded base64
			b, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
			if err != nil {
				return "", fmt.Errorf("pin is not valid base64 or hex")
			}
		}
		raw = b
	}
	if len(raw) != sha256.Size {
		return "", fmt.Errorf("pin must be %d bytes, got %d", sha256.Size, len(raw))
	}
	return Prefix + base64.StdEncoding.EncodeToString(raw), nil
}

// Verifier builds a tls.Config VerifyPeerCertificate function that accepts a
// leaf certificate only when its public key matches one of the pins.
func Verifier(pins ...string) (func([][]byte, [][]*x509.Certificate) error, error) {
	want := make([]string, 0, len(pins))
	for _, p := range pins {
		c, err := Normalize(p)
		if err != nil {
			return nil, err
		}
		want = append(want, c)
	}
	if len(want) == 0 {
		return nil, errors.New("no pins given")
	}
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("server presented no certificate")
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("parse server certificate: %w", err)
		}
		got := OfCert(leaf)
		for _, w := range want {
			if subtle.ConstantTimeCompare([]byte(got), []byte(w)) == 1 {
				return nil
			}
		}
		return fmt.Errorf("server key fingerprint %s does not match the expected pin", got)
	}, nil
}

// TLSConfig returns a client TLS config pinned to pin. Chain and hostname
// verification are replaced by the pin check, which is the point: a pinned
// server is usually reached by IP and has no CA-issued certificate.
func TLSConfig(pin string) (*tls.Config, error) {
	verify, err := Verifier(pin)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:            tls.VersionTLS12,
		InsecureSkipVerify:    true, // replaced by VerifyPeerCertificate below
		VerifyPeerCertificate: verify,
	}, nil
}

// HTTPClient returns an HTTP client for the control plane. With a pin it trusts
// exactly that server key; without one it uses the system trust store (the
// public-domain + ACME case).
func HTTPClient(pin string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if strings.TrimSpace(pin) == "" {
		return &http.Client{Timeout: timeout}, nil
	}
	cfg, err := TLSConfig(pin)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = cfg
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}
