package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"
)

const (
	defaultRenewBefore = 30 * 24 * time.Hour
	defaultCheckEvery  = 6 * time.Hour
)

// CertificateIssuer obtains a fresh certificate when the cached one needs
// renewal. ACMEClient implements this interface.
type CertificateIssuer interface {
	ObtainWildcard(termsOfServiceAgreed bool) (tls.Certificate, error)
}

// CertificateManagerConfig controls certificate renewal timing.
type CertificateManagerConfig struct {
	RenewBefore time.Duration
	CheckEvery  time.Duration
	TermsAgreed bool
}

// CertificateManager caches a certificate in memory and refreshes it before
// expiry. The manager never persists private keys or certificates to disk.
type CertificateManager struct {
	mu          sync.RWMutex
	issuer      CertificateIssuer
	config      CertificateManagerConfig
	certificate *tls.Certificate
	expiresAt   time.Time
}

// NewCertificateManager creates a manager for an ACME or test issuer.
func NewCertificateManager(issuer CertificateIssuer, config CertificateManagerConfig) (*CertificateManager, error) {
	if issuer == nil {
		return nil, fmt.Errorf("certificate issuer cannot be nil")
	}
	if config.RenewBefore <= 0 {
		config.RenewBefore = defaultRenewBefore
	}
	if config.CheckEvery <= 0 {
		config.CheckEvery = defaultCheckEvery
	}
	return &CertificateManager{issuer: issuer, config: config}, nil
}

// GetCertificate returns the cached certificate, obtaining or renewing it as
// needed. A still-valid cached certificate is retained if a renewal attempt
// fails temporarily.
func (m *CertificateManager) GetCertificate() (tls.Certificate, error) {
	now := time.Now()
	m.mu.RLock()
	certificate := cloneCertificate(m.certificate)
	expiresAt := m.expiresAt
	m.mu.RUnlock()
	if certificate != nil && now.Before(expiresAt.Add(-m.config.RenewBefore)) {
		return *certificate, nil
	}

	return m.refresh(certificate, expiresAt, now)
}

// TLSConfig returns a TLS configuration that loads the current certificate
// through tls.Config.GetCertificate.
func (m *CertificateManager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{ALPNProtocol},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			certificate, err := m.GetCertificate()
			if err != nil {
				return nil, err
			}
			return &certificate, nil
		},
	}
}

// Start refreshes the certificate immediately and supervises renewals until
// ctx is cancelled. It returns the initial refresh error, if any.
func (m *CertificateManager) Start(ctx context.Context) error {
	if _, err := m.GetCertificate(); err != nil {
		return err
	}
	ticker := time.NewTicker(m.config.CheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = m.GetCertificate()
		}
	}
}

func (m *CertificateManager) refresh(previous *tls.Certificate, expiresAt, now time.Time) (tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.certificate != nil && now.Before(m.expiresAt.Add(-m.config.RenewBefore)) {
		return *cloneCertificate(m.certificate), nil
	}

	certificate, err := m.issuer.ObtainWildcard(m.config.TermsAgreed)
	if err == nil {
		if err := populateLeaf(&certificate); err != nil {
			return tls.Certificate{}, err
		}
		m.certificate = cloneCertificate(&certificate)
		m.expiresAt = certificate.Leaf.NotAfter
		return certificate, nil
	}
	if previous != nil && now.Before(expiresAt) {
		return *previous, nil
	}
	return tls.Certificate{}, fmt.Errorf("obtain certificate: %w", err)
}

func populateLeaf(certificate *tls.Certificate) error {
	if certificate == nil || len(certificate.Certificate) == 0 {
		return fmt.Errorf("obtained certificate contained no certificate chain")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse certificate leaf: %w", err)
	}
	certificate.Leaf = leaf
	return nil
}

func cloneCertificate(certificate *tls.Certificate) *tls.Certificate {
	if certificate == nil {
		return nil
	}
	clone := *certificate
	clone.Certificate = make([][]byte, len(certificate.Certificate))
	for i, der := range certificate.Certificate {
		clone.Certificate[i] = append([]byte(nil), der...)
	}
	return &clone
}
