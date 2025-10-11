package security

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// BasicAuthValidator validates credentials.
type BasicAuthValidator interface {
	Validate(username, password string) bool
}

// FixedCredentialValidator validates against a fixed username and password using constant-time comparison.
type FixedCredentialValidator struct {
	Username string
	Password string
}

// NewFixedCredentialValidator creates a new FixedCredentialValidator.
func NewFixedCredentialValidator(username, password string) *FixedCredentialValidator {
	return &FixedCredentialValidator{
		Username: username,
		Password: password,
	}
}

// Validate checks if the username and password match using constant time comparison.
func (v *FixedCredentialValidator) Validate(username, password string) bool {
	uMatch := subtle.ConstantTimeCompare([]byte(username), []byte(v.Username)) == 1
	pMatch := subtle.ConstantTimeCompare([]byte(password), []byte(v.Password)) == 1
	return uMatch && pMatch
}

// MultiCredentialValidator validates against a map of authorized users.
type MultiCredentialValidator struct {
	credentials map[string]string // username -> password
}

// NewMultiCredentialValidator creates a validator from a map of user->pass.
func NewMultiCredentialValidator(creds map[string]string) *MultiCredentialValidator {
	copyMap := make(map[string]string, len(creds))
	for k, v := range creds {
		copyMap[k] = v
	}
	return &MultiCredentialValidator{credentials: copyMap}
}

// Validate checks if the given credentials are valid.
func (m *MultiCredentialValidator) Validate(username, password string) bool {
	expectedPass, ok := m.credentials[username]
	if !ok {
		// Run a dummy constant time comparison to avoid timing leak of user existence
		subtle.ConstantTimeCompare([]byte(password), []byte("dummy-password-never-matches"))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(expectedPass)) == 1
}

// BasicAuthMiddlewareConfig configures the BasicAuth middleware.
type BasicAuthMiddlewareConfig struct {
	Validator BasicAuthValidator
	Realm     string
}

// BasicAuthMiddleware returns an HTTP middleware enforcing HTTP Basic Authentication.
func BasicAuthMiddleware(cfg BasicAuthMiddlewareConfig) func(http.Handler) http.Handler {
	realm := cfg.Realm
	if realm == "" {
		realm = "Portal"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				unauthorized(w, realm)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
				unauthorized(w, realm)
				return
			}

			payload, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				unauthorized(w, realm)
				return
			}

			pair := strings.SplitN(string(payload), ":", 2)
			if len(pair) != 2 {
				unauthorized(w, realm)
				return
			}

			username, password := pair[0], pair[1]
			if cfg.Validator == nil || !cfg.Validator.Validate(username, password) {
				unauthorized(w, realm)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
