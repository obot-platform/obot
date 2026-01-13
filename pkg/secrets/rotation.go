package secrets

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CookieSecretVersion tracks which secret version is active for writes
	CookieSecretVersion = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obot_auth_cookie_secret_version",
			Help: "Current cookie secret version by provider (increments on rotation)",
		},
		[]string{"provider"},
	)

	// SecretRotationAge tracks how long the current secret has been in use
	SecretRotationAge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obot_auth_secret_rotation_age_seconds",
			Help: "Age of the current cookie secret in seconds by provider",
		},
		[]string{"provider"},
	)

	// CookieDecryptByVersion tracks successful cookie decryptions by secret version
	CookieDecryptByVersion = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_cookie_decrypt_by_version_total",
			Help: "Total successful cookie decryptions by provider and secret version",
		},
		[]string{"provider", "version"},
	)
)

// RotationConfig holds secret rotation configuration
type RotationConfig struct {
	// Provider name for metrics
	Provider string

	// CurrentSecret is the active secret used for encrypting new cookies
	CurrentSecret []byte

	// PreviousSecrets are older secrets accepted for decryption (ordered newest to oldest)
	PreviousSecrets [][]byte

	// SecretVersion is the current secret version (increments on rotation)
	SecretVersion int

	// RotationStartTime is when the current secret was activated
	RotationStartTime time.Time

	// GracePeriod is the recommended time to wait before removing old secrets
	// Default: 7 days (max session lifetime)
	GracePeriod time.Duration
}

// LoadRotationConfig loads secret rotation configuration from environment variables
func LoadRotationConfig(provider string) (*RotationConfig, error) {
	config := &RotationConfig{
		Provider:          provider,
		SecretVersion:     1,
		RotationStartTime: time.Now(),
		GracePeriod:       7 * 24 * time.Hour, // 7 days default
	}

	// Load current secret (required)
	currentSecretKey := fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_COOKIE_SECRET", strings.ToUpper(provider))
	currentSecretStr := os.Getenv(currentSecretKey)
	if currentSecretStr == "" {
		// Fallback to shared secret
		currentSecretStr = os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_SECRET")
	}

	if currentSecretStr == "" {
		return nil, fmt.Errorf("cookie secret not configured: set %s or OBOT_AUTH_PROVIDER_COOKIE_SECRET", currentSecretKey)
	}

	// Decode and validate current secret
	currentSecret, err := base64.StdEncoding.DecodeString(currentSecretStr)
	if err != nil {
		return nil, fmt.Errorf("current cookie secret must be valid base64: %w", err)
	}

	if len(currentSecret) < 32 {
		return nil, fmt.Errorf("current cookie secret must be at least 32 bytes (256 bits), got %d bytes", len(currentSecret))
	}

	config.CurrentSecret = currentSecret

	// Load previous secrets (optional, comma-separated)
	previousSecretsKey := fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS", strings.ToUpper(provider))
	previousSecretsStr := os.Getenv(previousSecretsKey)
	if previousSecretsStr == "" {
		// Fallback to shared previous secrets
		previousSecretsStr = os.Getenv("OBOT_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS")
	}

	if previousSecretsStr != "" {
		secretStrs := strings.Split(previousSecretsStr, ",")
		for i, secretStr := range secretStrs {
			secretStr = strings.TrimSpace(secretStr)
			if secretStr == "" {
				continue
			}

			previousSecret, err := base64.StdEncoding.DecodeString(secretStr)
			if err != nil {
				return nil, fmt.Errorf("previous cookie secret %d must be valid base64: %w", i+1, err)
			}

			if len(previousSecret) < 32 {
				return nil, fmt.Errorf("previous cookie secret %d must be at least 32 bytes, got %d bytes", i+1, len(previousSecret))
			}

			config.PreviousSecrets = append(config.PreviousSecrets, previousSecret)
		}

		// Secret version is 1 (current) + number of previous secrets
		config.SecretVersion = 1 + len(config.PreviousSecrets)
	}

	// Load grace period if specified
	if val := os.Getenv("OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD"); val != "" {
		duration, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("invalid OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD: %w", err)
		}
		config.GracePeriod = duration
	}

	// Initialize metrics
	CookieSecretVersion.WithLabelValues(provider).Set(float64(config.SecretVersion))
	SecretRotationAge.WithLabelValues(provider).Set(0)

	return config, nil
}

// GetEncryptSecret returns the secret to use for encryption (always current secret)
func (c *RotationConfig) GetEncryptSecret() []byte {
	return c.CurrentSecret
}

// TryDecrypt attempts to decrypt data using the current secret and previous secrets
// Returns the decrypted data and the version index used (0 = current, 1+ = previous)
func (c *RotationConfig) TryDecrypt(encryptedData []byte, decryptFunc func([]byte, []byte) ([]byte, error)) ([]byte, int, error) {
	// Try current secret first (most common case)
	if decrypted, err := decryptFunc(encryptedData, c.CurrentSecret); err == nil {
		CookieDecryptByVersion.WithLabelValues(c.Provider, "current").Inc()
		return decrypted, 0, nil
	}

	// Try previous secrets in order (newest to oldest)
	for i, previousSecret := range c.PreviousSecrets {
		if decrypted, err := decryptFunc(encryptedData, previousSecret); err == nil {
			versionLabel := fmt.Sprintf("previous-%d", i+1)
			CookieDecryptByVersion.WithLabelValues(c.Provider, versionLabel).Inc()
			return decrypted, i + 1, nil
		}
	}

	// All secrets failed
	return nil, -1, fmt.Errorf("failed to decrypt with current or previous secrets")
}

// ValidateSecret ensures a secret meets minimum entropy requirements
func (c *RotationConfig) ValidateSecret(secret []byte) error {
	if len(secret) < 32 {
		return fmt.Errorf("secret must be at least 32 bytes (256 bits), got %d bytes", len(secret))
	}
	return nil
}

// SecureCompare performs constant-time comparison of two secrets
func (c *RotationConfig) SecureCompare(secret1, secret2 []byte) bool {
	return subtle.ConstantTimeCompare(secret1, secret2) == 1
}

// IsInGracePeriod checks if we're still within the grace period after rotation
func (c *RotationConfig) IsInGracePeriod() bool {
	return time.Since(c.RotationStartTime) < c.GracePeriod
}

// GetRotationAge returns how long the current secret has been active
func (c *RotationConfig) GetRotationAge() time.Duration {
	return time.Since(c.RotationStartTime)
}

// UpdateRotationMetrics updates the rotation age metric
func (c *RotationConfig) UpdateRotationMetrics() {
	age := c.GetRotationAge()
	SecretRotationAge.WithLabelValues(c.Provider).Set(age.Seconds())
}

// GenerateSecret generates a cryptographically secure 32-byte secret
func GenerateSecret() (string, error) {
	return GenerateSecretWithLength(32)
}

// GenerateSecretWithLength generates a cryptographically secure secret of specified length
func GenerateSecretWithLength(length int) (string, error) {
	if length < 32 {
		return "", fmt.Errorf("secret length must be at least 32 bytes")
	}

	secret := make([]byte, length)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}

	return base64.StdEncoding.EncodeToString(secret), nil
}

// ValidateRotationState checks if the rotation configuration is valid
func (c *RotationConfig) ValidateRotationState() error {
	// Validate current secret
	if err := c.ValidateSecret(c.CurrentSecret); err != nil {
		return fmt.Errorf("invalid current secret: %w", err)
	}

	// Validate previous secrets
	for i, previousSecret := range c.PreviousSecrets {
		if err := c.ValidateSecret(previousSecret); err != nil {
			return fmt.Errorf("invalid previous secret %d: %w", i+1, err)
		}

		// Ensure previous secret is different from current
		if c.SecureCompare(c.CurrentSecret, previousSecret) {
			return fmt.Errorf("previous secret %d matches current secret (duplicate detected)", i+1)
		}

		// Ensure no duplicate previous secrets
		for j := 0; j < i; j++ {
			if c.SecureCompare(previousSecret, c.PreviousSecrets[j]) {
				return fmt.Errorf("previous secrets %d and %d are identical (duplicate detected)", j+1, i+1)
			}
		}
	}

	return nil
}

// GetSecretCount returns the total number of active secrets (current + previous)
func (c *RotationConfig) GetSecretCount() int {
	return 1 + len(c.PreviousSecrets)
}

// String returns a human-readable representation of the rotation config
func (c *RotationConfig) String() string {
	previousCount := len(c.PreviousSecrets)
	if previousCount == 0 {
		return fmt.Sprintf("secret rotation: version=%d, current secret only, age=%v",
			c.SecretVersion, c.GetRotationAge().Round(time.Hour))
	}

	return fmt.Sprintf("secret rotation: version=%d, %d previous secret(s), age=%v, grace_period=%v",
		c.SecretVersion, previousCount, c.GetRotationAge().Round(time.Hour), c.GracePeriod)
}
