// Package keys provides WireGuard-style Curve25519 (X25519) node keys.
//
// Uses the Go standard library (crypto/ecdh) so M1a needs no external
// dependencies. The tunnel data plane (wireguard-go) arrives in M1b.
package keys

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
)

// Private is an X25519 private (node) key.
type Private struct{ k *ecdh.PrivateKey }

// Public is an X25519 public key.
type Public struct{ k *ecdh.PublicKey }

// Generate creates a new random node keypair.
func Generate() (Private, error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Private{}, err
	}
	return Private{k}, nil
}

// Public returns the public key for this private key.
func (p Private) Public() Public { return Public{p.k.PublicKey()} }

// String encodes the private key as standard base64.
func (p Private) String() string { return base64.StdEncoding.EncodeToString(p.k.Bytes()) }

// String encodes the public key as standard base64.
func (p Public) String() string { return base64.StdEncoding.EncodeToString(p.k.Bytes()) }

// ParsePrivate decodes a base64 private key.
func ParsePrivate(s string) (Private, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Private{}, err
	}
	k, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return Private{}, err
	}
	return Private{k}, nil
}

// ParsePublic decodes a base64 public key.
func ParsePublic(s string) (Public, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Public{}, err
	}
	k, err := ecdh.X25519().NewPublicKey(b)
	if err != nil {
		return Public{}, err
	}
	return Public{k}, nil
}
