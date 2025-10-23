package transport

import (
	"context"
	"crypto/tls"
	"sync"
	"testing"
	"time"
)

type countingIssuer struct {
	mu          sync.Mutex
	certificate tls.Certificate
	err         error
	count       int
}

func (i *countingIssuer) ObtainWildcard(bool) (tls.Certificate, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.count++
	return i.certificate, i.err
}

func (i *countingIssuer) calls() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.count
}

func TestCertificateManagerCachesCertificate(t *testing.T) {
	certificate, err := GenerateSelfSignedCert([]string{"*.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	issuer := &countingIssuer{certificate: certificate}
	manager, err := NewCertificateManager(issuer, CertificateManagerConfig{})
	if err != nil {
		t.Fatal(err)
	}

	first, err := manager.GetCertificate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.GetCertificate()
	if err != nil {
		t.Fatal(err)
	}
	if issuer.calls() != 1 {
		t.Fatalf("issuer calls = %d, want 1", issuer.calls())
	}
	if len(first.Certificate) != len(second.Certificate) {
		t.Fatal("cached certificate chain changed")
	}
}

func TestCertificateManagerRenewsBeforeExpiryAndKeepsValidCacheOnFailure(t *testing.T) {
	certificate, err := GenerateSelfSignedCert([]string{"*.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	issuer := &countingIssuer{certificate: certificate}
	manager, err := NewCertificateManager(issuer, CertificateManagerConfig{RenewBefore: 400 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GetCertificate(); err != nil {
		t.Fatal(err)
	}

	issuer.mu.Lock()
	issuer.err = errTestIssuer
	issuer.mu.Unlock()
	if _, err := manager.GetCertificate(); err != nil {
		t.Fatalf("renewal failure should retain valid certificate: %v", err)
	}
	if issuer.calls() != 2 {
		t.Fatalf("issuer calls = %d, want 2", issuer.calls())
	}
}

func TestCertificateManagerStartAndTLSConfig(t *testing.T) {
	certificate, err := GenerateSelfSignedCert([]string{"*.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	issuer := &countingIssuer{certificate: certificate}
	manager, err := NewCertificateManager(issuer, CertificateManagerConfig{CheckEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- manager.Start(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if manager.TLSConfig().GetCertificate == nil {
		t.Fatal("TLSConfig() did not configure GetCertificate")
	}
}

var errTestIssuer = &testIssuerError{}

type testIssuerError struct{}

func (*testIssuerError) Error() string { return "issuer unavailable" }
