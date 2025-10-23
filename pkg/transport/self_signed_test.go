package transport

import (
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateSelfSignedWildcardCert(t *testing.T) {
	certificate, err := GenerateSelfSignedWildcardCert("*.tunnel.example.com.")
	if err != nil {
		t.Fatalf("GenerateSelfSignedWildcardCert() error = %v", err)
	}
	if len(certificate.Certificate) != 1 || certificate.PrivateKey == nil {
		t.Fatal("generated certificate did not include a certificate and private key")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "*.tunnel.example.com" {
		t.Errorf("common name = %q, want wildcard", leaf.Subject.CommonName)
	}
	if got, want := leaf.DNSNames, []string{"*.tunnel.example.com", "tunnel.example.com"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("DNS names = %v, want %v", got, want)
	}
	if !leaf.NotAfter.After(time.Now().Add(364 * 24 * time.Hour)) {
		t.Errorf("certificate expires too soon: %s", leaf.NotAfter)
	}
	if leaf.IsCA || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("certificate usage is not server-only: IsCA=%v usages=%v", leaf.IsCA, leaf.ExtKeyUsage)
	}
}

func TestGenerateSelfSignedWildcardCertRejectsInvalidDomain(t *testing.T) {
	for _, domain := range []string{"localhost", "", "https://example.com", "example.com/path"} {
		if _, err := GenerateSelfSignedWildcardCert(domain); err == nil {
			t.Errorf("GenerateSelfSignedWildcardCert(%q) succeeded, want error", domain)
		}
	}
}
