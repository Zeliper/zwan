package tlsconf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Zeliper/zwan/shared/certpin"
)

const (
	keyFile     = "server.key"
	certFile    = "server.crt"
	certLife    = 10 * 365 * 24 * time.Hour
	renewBefore = 30 * 24 * time.Hour
)

// selfSigned returns the persisted self-signed certificate for dir, generating
// the key on first use and re-issuing the certificate when it is missing,
// expiring, or no longer covers sans.
//
// The key is generated exactly once and reused across re-issues, so the SPKI pin
// handed out to clients stays valid for the life of the installation.
func selfSigned(dir string, sans []string) (tls.Certificate, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("tls state dir: %w", err)
	}
	key, err := loadOrCreateKey(filepath.Join(dir, keyFile))
	if err != nil {
		return tls.Certificate{}, "", err
	}
	spki, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return tls.Certificate{}, "", err
	}
	pin := certpin.OfSPKI(spki)

	dnsNames, ips := splitSANs(sans)
	certPath := filepath.Join(dir, certFile)
	if der, err := readCertDER(certPath); err == nil {
		if c, err := x509.ParseCertificate(der); err == nil && certUsable(c, key, dnsNames, ips) {
			return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: c}, pin, nil
		}
	}

	der, err := issue(key, dnsNames, ips)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return tls.Certificate{}, "", err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pin, nil
}

func loadOrCreateKey(path string) (*ecdsa.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		if blk, _ := pem.Decode(b); blk != nil {
			if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
				if ec, ok := k.(*ecdsa.PrivateKey); ok {
					return ec, nil
				}
			}
		}
		return nil, fmt.Errorf("%s exists but is not a usable EC private key; move it aside to regenerate", path)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writePEM(path, "PRIVATE KEY", der, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func issue(key *ecdsa.PrivateKey, dnsNames []string, ips []net.IP) ([]byte, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	cn := "zwan control plane"
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"zwan"}},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(certLife),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}
	return x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
}

// certUsable reports whether an existing certificate can be kept: same key, not
// close to expiry, and covering every requested SAN.
func certUsable(c *x509.Certificate, key *ecdsa.PrivateKey, dnsNames []string, ips []net.IP) bool {
	pub, ok := c.PublicKey.(*ecdsa.PublicKey)
	if !ok || !pub.Equal(key.Public()) {
		return false
	}
	if time.Now().Add(renewBefore).After(c.NotAfter) {
		return false
	}
	for _, want := range dnsNames {
		if !containsString(c.DNSNames, want) {
			return false
		}
	}
	for _, want := range ips {
		if !containsIP(c.IPAddresses, want) {
			return false
		}
	}
	return true
}

// splitSANs sorts host entries into DNS names and IP addresses, dropping
// duplicates and empty values.
func splitSANs(sans []string) ([]string, []net.IP) {
	var dnsNames []string
	var ips []net.IP
	for _, s := range sans {
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			if !containsIP(ips, ip) {
				ips = append(ips, ip)
			}
			continue
		}
		if !containsString(dnsNames, s) {
			dnsNames = append(dnsNames, s)
		}
	}
	return dnsNames, ips
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func containsIP(list []net.IP, want net.IP) bool {
	for _, ip := range list {
		if ip.Equal(want) {
			return true
		}
	}
	return false
}

func readCertDER(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil || blk.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s is not a PEM certificate", path)
	}
	return blk.Bytes, nil
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	return os.WriteFile(path, buf, mode)
}
