package registry

import (
	"errors"
	"strings"
	"testing"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		expectErr bool
	}{
		{"Valid simple name", "myapp", "myapp", false},
		{"Valid with hyphens", "my-test-app-1", "my-test-app-1", false},
		{"Uppercase normalization", "MyApp123", "myapp123", false},
		{"Single character", "a", "a", false},
		{"Empty string", "", "", true},
		{"Leading hyphen", "-myapp", "", true},
		{"Trailing hyphen", "myapp-", "", true},
		{"Special characters", "my_app", "", true},
		{"Dots invalid", "my.app", "", true},
		{"Reserved name admin", "admin", "", true},
		{"Reserved name api", "api", "", true},
		{"Reserved name www", "www", "", true},
		{"Reserved name healthz", "healthz", "", true},
		{"Too long (>63 chars)", strings.Repeat("a", 64), "", true},
		{"Max length (63 chars)", strings.Repeat("a", 63), strings.Repeat("a", 63), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateSubdomain(tt.input)
			if (err != nil) != tt.expectErr {
				t.Fatalf("ValidateSubdomain(%q) error = %v, expectErr = %v", tt.input, err, tt.expectErr)
			}
			if tt.expectErr {
				if !errors.Is(err, portalErr.ErrSubdomainInvalid) {
					t.Errorf("expected error wrapping ErrSubdomainInvalid, got: %v", err)
				}
			} else {
				if got != tt.want {
					t.Errorf("ValidateSubdomain(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}
