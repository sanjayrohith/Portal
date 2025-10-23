package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// GenerateSelfSignedWildcardCert creates an offline ECDSA server certificate
// for *.domain and domain. It is intended for local testing and staging, not
// public production traffic.
func GenerateSelfSignedWildcardCert(domain string) (tls.Certificate, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	domain = strings.TrimPrefix(domain, "*.")
	if domain == "" || strings.ContainsAny(domain, "/:@ ") || !strings.Contains(domain, ".") {
		return tls.Certificate{}, fmt.Errorf("invalid wildcard domain %q", domain)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed private key: %w", err)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed serial number: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "*." + domain,
			Organization: []string{"Portal Local Testing"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"*." + domain, domain},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create self-signed wildcard certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey}, nil
}
