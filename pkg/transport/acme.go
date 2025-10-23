package transport

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	lego "github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// ACMEConfig configures wildcard certificate issuance through ACME DNS-01.
type ACMEConfig struct {
	Email                string
	Domain               string
	DirectoryURL         string
	TermsOfServiceAgreed bool
	CertificateTimeout   time.Duration
}

// ACMEClient wraps lego's ACME and DNS-01 challenge clients for Portal.
type ACMEClient struct {
	client *lego.Client
	domain string
}

type acmeUser struct {
	email        string
	privateKey   *ecdsa.PrivateKey
	registration *registration.Resource
}

func (u *acmeUser) GetEmail() string { return u.email }

func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }

func (u *acmeUser) GetPrivateKey() crypto.PrivateKey { return u.privateKey }

// NewACMEClient creates a DNS-01 ACME client. The provider supplies the DNS
// credentials and is intentionally injected so operators can choose any lego
// supported DNS provider without storing credentials in Portal.
func NewACMEClient(config ACMEConfig, provider challenge.Provider) (*ACMEClient, error) {
	domain, err := wildcardDomain(config.Domain)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("ACME DNS-01 provider cannot be nil")
	}
	if strings.TrimSpace(config.Email) == "" {
		return nil, fmt.Errorf("ACME account email cannot be empty")
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ACME account key: %w", err)
	}
	user := &acmeUser{email: strings.TrimSpace(config.Email), privateKey: privateKey}
	legoConfig := lego.NewConfig(user)
	if config.DirectoryURL != "" {
		legoConfig.CADirURL = config.DirectoryURL
	}
	legoConfig.Certificate.KeyType = certcrypto.EC256
	if config.CertificateTimeout > 0 {
		legoConfig.Certificate.Timeout = config.CertificateTimeout
	}
	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return nil, fmt.Errorf("create ACME client: %w", err)
	}
	if err := client.Challenge.SetDNS01Provider(provider); err != nil {
		return nil, fmt.Errorf("configure DNS-01 provider: %w", err)
	}
	return &ACMEClient{client: client, domain: domain}, nil
}

// ObtainWildcard registers the ACME account and obtains a wildcard
// certificate for the configured domain using DNS-01.
func (c *ACMEClient) ObtainWildcard(termsOfServiceAgreed bool) (tls.Certificate, error) {
	if c == nil || c.client == nil {
		return tls.Certificate{}, fmt.Errorf("ACME client is nil")
	}
	if _, err := c.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: termsOfServiceAgreed}); err != nil {
		return tls.Certificate{}, fmt.Errorf("register ACME account: %w", err)
	}
	resource, err := c.client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{"*." + c.domain},
		Bundle:  true,
	})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("obtain wildcard certificate: %w", err)
	}
	cert, err := tls.X509KeyPair(resource.Certificate, resource.PrivateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse obtained wildcard certificate: %w", err)
	}
	return cert, nil
}

func wildcardDomain(domain string) (string, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	domain = strings.TrimPrefix(domain, "*.")
	if domain == "" || strings.ContainsAny(domain, "/:@ ") || !strings.Contains(domain, ".") {
		return "", fmt.Errorf("invalid ACME wildcard domain %q", domain)
	}
	return domain, nil
}
