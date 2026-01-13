package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const (
	// MinSecretBits is the minimum required entropy for cookie secrets (256 bits)
	MinSecretBits = 256
	// MinSecretBytes is the minimum required bytes for cookie secrets (32 bytes)
	MinSecretBytes = MinSecretBits / 8
)

// ValidateCookieSecret ensures the cookie secret has sufficient entropy.
// The secret must be base64-encoded and decode to at least 32 bytes (256 bits).
func ValidateCookieSecret(base64Secret string) error {
	if base64Secret == "" {
		return fmt.Errorf("cookie secret is required")
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Secret)
	if err != nil {
		return fmt.Errorf("cookie secret must be valid base64: %w", err)
	}

	if len(decoded) < MinSecretBytes {
		return fmt.Errorf("cookie secret must be at least %d bytes (%d bits), got %d bytes",
			MinSecretBytes, MinSecretBits, len(decoded))
	}

	return nil
}

// GenerateCookieSecret generates a cryptographically secure cookie secret.
// Returns a base64-encoded 32-byte (256-bit) random value.
func GenerateCookieSecret() (string, error) {
	secret := make([]byte, MinSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}
