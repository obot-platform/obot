package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateCookieSecret(t *testing.T) {
	tests := []struct {
		name        string
		secret      string
		expectError bool
		errorContains string
	}{
		{
			name:        "empty secret",
			secret:      "",
			expectError: true,
			errorContains: "cookie secret is required",
		},
		{
			name:        "invalid base64",
			secret:      "not-valid-base64!@#$",
			expectError: true,
			errorContains: "must be valid base64",
		},
		{
			name:        "valid 32-byte secret",
			secret:      base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))),
			expectError: false,
		},
		{
			name:        "valid 64-byte secret",
			secret:      base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 64))),
			expectError: false,
		},
		{
			name:        "too short - 31 bytes",
			secret:      base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 31))),
			expectError: true,
			errorContains: "must be at least 32 bytes (256 bits), got 31 bytes",
		},
		{
			name:        "too short - 16 bytes",
			secret:      base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 16))),
			expectError: true,
			errorContains: "must be at least 32 bytes (256 bits), got 16 bytes",
		},
		{
			name:        "too short - 1 byte",
			secret:      base64.StdEncoding.EncodeToString([]byte("a")),
			expectError: true,
			errorContains: "must be at least 32 bytes (256 bits), got 1 bytes",
		},
		{
			name:        "generated secret is valid",
			secret:      mustGenerateCookieSecret(t),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookieSecret(tt.secret)

			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateCookieSecret() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ValidateCookieSecret() error = %v, want error containing %q", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCookieSecret() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestGenerateCookieSecret(t *testing.T) {
	// Test that GenerateCookieSecret produces valid secrets
	for i := 0; i < 10; i++ {
		secret, err := GenerateCookieSecret()
		if err != nil {
			t.Fatalf("GenerateCookieSecret() error = %v", err)
		}

		// Verify it's valid base64
		decoded, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			t.Errorf("GenerateCookieSecret() produced invalid base64: %v", err)
		}

		// Verify it's exactly 32 bytes
		if len(decoded) != MinSecretBytes {
			t.Errorf("GenerateCookieSecret() produced %d bytes, want %d bytes", len(decoded), MinSecretBytes)
		}

		// Verify it passes validation
		if err := ValidateCookieSecret(secret); err != nil {
			t.Errorf("GenerateCookieSecret() produced invalid secret: %v", err)
		}
	}
}

func TestGenerateCookieSecret_Uniqueness(t *testing.T) {
	// Generate multiple secrets and verify they're unique
	secrets := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		secret, err := GenerateCookieSecret()
		if err != nil {
			t.Fatalf("GenerateCookieSecret() error = %v", err)
		}

		if secrets[secret] {
			t.Errorf("GenerateCookieSecret() produced duplicate secret on iteration %d", i)
		}
		secrets[secret] = true
	}

	if len(secrets) != iterations {
		t.Errorf("GenerateCookieSecret() produced %d unique secrets, want %d", len(secrets), iterations)
	}
}

// mustGenerateCookieSecret is a test helper that generates a secret or fails the test
func mustGenerateCookieSecret(t *testing.T) string {
	secret, err := GenerateCookieSecret()
	if err != nil {
		t.Fatalf("GenerateCookieSecret() error = %v", err)
	}
	return secret
}
