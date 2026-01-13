package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadRotationConfig(t *testing.T) {
	// Generate valid secrets for testing
	secret1, _ := GenerateSecret()
	secret2, _ := GenerateSecret()
	secret3, _ := GenerateSecret()

	tests := []struct {
		name              string
		currentSecret     string
		previousSecrets   string
		provider          string
		expectError       bool
		expectedVersion   int
		expectedPrevCount int
	}{
		{
			name:              "current secret only",
			currentSecret:     secret1,
			previousSecrets:   "",
			provider:          "entra",
			expectError:       false,
			expectedVersion:   1,
			expectedPrevCount: 0,
		},
		{
			name:              "current + one previous",
			currentSecret:     secret1,
			previousSecrets:   secret2,
			provider:          "entra",
			expectError:       false,
			expectedVersion:   2,
			expectedPrevCount: 1,
		},
		{
			name:              "current + two previous",
			currentSecret:     secret1,
			previousSecrets:   fmt.Sprintf("%s,%s", secret2, secret3),
			provider:          "entra",
			expectError:       false,
			expectedVersion:   3,
			expectedPrevCount: 2,
		},
		{
			name:          "no current secret",
			currentSecret: "",
			provider:      "entra",
			expectError:   true,
		},
		{
			name:          "invalid base64",
			currentSecret: "not-valid-base64!!!",
			provider:      "entra",
			expectError:   true,
		},
		{
			name:          "secret too short",
			currentSecret: base64.StdEncoding.EncodeToString([]byte("tooshort")),
			provider:      "entra",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.currentSecret != "" {
				os.Setenv(fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_COOKIE_SECRET", strings.ToUpper(tt.provider)), tt.currentSecret)
				defer os.Unsetenv(fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_COOKIE_SECRET", strings.ToUpper(tt.provider)))
			}
			if tt.previousSecrets != "" {
				os.Setenv(fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS", strings.ToUpper(tt.provider)), tt.previousSecrets)
				defer os.Unsetenv(fmt.Sprintf("OBOT_%s_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS", strings.ToUpper(tt.provider)))
			}

			config, err := LoadRotationConfig(tt.provider)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if config.SecretVersion != tt.expectedVersion {
				t.Errorf("expected version %d, got %d", tt.expectedVersion, config.SecretVersion)
			}

			if len(config.PreviousSecrets) != tt.expectedPrevCount {
				t.Errorf("expected %d previous secrets, got %d", tt.expectedPrevCount, len(config.PreviousSecrets))
			}

			if config.Provider != tt.provider {
				t.Errorf("expected provider %s, got %s", tt.provider, config.Provider)
			}
		})
	}
}

func TestGetEncryptSecret(t *testing.T) {
	secret1, _ := GenerateSecret()
	secret2, _ := GenerateSecret()

	os.Setenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET", secret1)
	os.Setenv("OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS", secret2)
	defer os.Unsetenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET")
	defer os.Unsetenv("OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS")

	config, err := LoadRotationConfig("entra")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	encryptSecret := config.GetEncryptSecret()

	// Should always return current secret
	currentSecretDecoded, _ := base64.StdEncoding.DecodeString(secret1)
	if !config.SecureCompare(encryptSecret, currentSecretDecoded) {
		t.Error("GetEncryptSecret should return current secret")
	}
}

func TestTryDecrypt(t *testing.T) {
	secret1, _ := GenerateSecret()
	secret2, _ := GenerateSecret()
	secret3, _ := GenerateSecret()

	os.Setenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET", secret1)
	os.Setenv("OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS", fmt.Sprintf("%s,%s", secret2, secret3))
	defer os.Unsetenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET")
	defer os.Unsetenv("OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS")

	config, err := LoadRotationConfig("entra")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Mock decrypt function that succeeds if data matches secret
	mockDecrypt := func(data []byte, secret []byte) ([]byte, error) {
		if string(data) == string(secret) {
			return []byte("decrypted"), nil
		}
		return nil, errors.New("decryption failed")
	}

	t.Run("decrypt with current secret", func(t *testing.T) {
		encryptedData := config.CurrentSecret
		decrypted, version, err := config.TryDecrypt(encryptedData, mockDecrypt)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if version != 0 {
			t.Errorf("expected version 0 (current), got %d", version)
		}

		if string(decrypted) != "decrypted" {
			t.Error("decryption failed")
		}
	})

	t.Run("decrypt with first previous secret", func(t *testing.T) {
		encryptedData := config.PreviousSecrets[0]
		decrypted, version, err := config.TryDecrypt(encryptedData, mockDecrypt)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if version != 1 {
			t.Errorf("expected version 1 (previous-1), got %d", version)
		}

		if string(decrypted) != "decrypted" {
			t.Error("decryption failed")
		}
	})

	t.Run("decrypt with second previous secret", func(t *testing.T) {
		encryptedData := config.PreviousSecrets[1]
		decrypted, version, err := config.TryDecrypt(encryptedData, mockDecrypt)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if version != 2 {
			t.Errorf("expected version 2 (previous-2), got %d", version)
		}

		if string(decrypted) != "decrypted" {
			t.Error("decryption failed")
		}
	})

	t.Run("decrypt with unknown secret fails", func(t *testing.T) {
		encryptedData := []byte("unknown-secret")
		_, version, err := config.TryDecrypt(encryptedData, mockDecrypt)

		if err == nil {
			t.Error("expected error but got nil")
		}

		if version != -1 {
			t.Errorf("expected version -1 on failure, got %d", version)
		}
	})
}

func TestValidateSecret(t *testing.T) {
	config := &RotationConfig{}

	tests := []struct {
		name        string
		secret      []byte
		expectError bool
	}{
		{
			name:        "32-byte secret valid",
			secret:      make([]byte, 32),
			expectError: false,
		},
		{
			name:        "64-byte secret valid",
			secret:      make([]byte, 64),
			expectError: false,
		},
		{
			name:        "31-byte secret too short",
			secret:      make([]byte, 31),
			expectError: true,
		},
		{
			name:        "16-byte secret too short",
			secret:      make([]byte, 16),
			expectError: true,
		},
		{
			name:        "empty secret invalid",
			secret:      []byte{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateSecret(tt.secret)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSecureCompare(t *testing.T) {
	config := &RotationConfig{}

	secret1 := []byte("secret123")
	secret2 := []byte("secret123")
	secret3 := []byte("different")

	if !config.SecureCompare(secret1, secret2) {
		t.Error("identical secrets should compare equal")
	}

	if config.SecureCompare(secret1, secret3) {
		t.Error("different secrets should not compare equal")
	}
}

func TestGenerateSecret(t *testing.T) {
	t.Run("generates valid 32-byte secret", func(t *testing.T) {
		secret, err := GenerateSecret()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		decoded, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			t.Error("generated secret should be valid base64")
		}

		if len(decoded) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(decoded))
		}
	})

	t.Run("generates unique secrets", func(t *testing.T) {
		secret1, _ := GenerateSecret()
		secret2, _ := GenerateSecret()

		if secret1 == secret2 {
			t.Error("generated secrets should be unique")
		}
	})

	t.Run("custom length", func(t *testing.T) {
		secret, err := GenerateSecretWithLength(64)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		decoded, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			t.Error("generated secret should be valid base64")
		}

		if len(decoded) != 64 {
			t.Errorf("expected 64 bytes, got %d", len(decoded))
		}
	})

	t.Run("rejects length < 32", func(t *testing.T) {
		_, err := GenerateSecretWithLength(16)
		if err == nil {
			t.Error("expected error for length < 32")
		}
	})
}

func TestValidateRotationState(t *testing.T) {
	validSecret1, _ := GenerateSecret()
	validSecret2, _ := GenerateSecret()
	validSecret3, _ := GenerateSecret()

	secret1Decoded, _ := base64.StdEncoding.DecodeString(validSecret1)
	secret2Decoded, _ := base64.StdEncoding.DecodeString(validSecret2)
	secret3Decoded, _ := base64.StdEncoding.DecodeString(validSecret3)

	tests := []struct {
		name            string
		currentSecret   []byte
		previousSecrets [][]byte
		expectError     bool
		errorContains   string
	}{
		{
			name:            "valid single secret",
			currentSecret:   secret1Decoded,
			previousSecrets: nil,
			expectError:     false,
		},
		{
			name:            "valid with previous secrets",
			currentSecret:   secret1Decoded,
			previousSecrets: [][]byte{secret2Decoded, secret3Decoded},
			expectError:     false,
		},
		{
			name:          "current secret too short",
			currentSecret: []byte("short"),
			expectError:   true,
			errorContains: "invalid current secret",
		},
		{
			name:            "previous secret too short",
			currentSecret:   secret1Decoded,
			previousSecrets: [][]byte{[]byte("short")},
			expectError:     true,
			errorContains:   "invalid previous secret",
		},
		{
			name:            "duplicate current and previous",
			currentSecret:   secret1Decoded,
			previousSecrets: [][]byte{secret1Decoded},
			expectError:     true,
			errorContains:   "matches current secret",
		},
		{
			name:            "duplicate previous secrets",
			currentSecret:   secret1Decoded,
			previousSecrets: [][]byte{secret2Decoded, secret2Decoded},
			expectError:     true,
			errorContains:   "are identical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RotationConfig{
				CurrentSecret:   tt.currentSecret,
				PreviousSecrets: tt.previousSecrets,
			}

			err := config.ValidateRotationState()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsInGracePeriod(t *testing.T) {
	t.Run("within grace period", func(t *testing.T) {
		config := &RotationConfig{
			RotationStartTime: time.Now().Add(-1 * time.Hour),
			GracePeriod:       24 * time.Hour,
		}

		if !config.IsInGracePeriod() {
			t.Error("should be within grace period")
		}
	})

	t.Run("past grace period", func(t *testing.T) {
		config := &RotationConfig{
			RotationStartTime: time.Now().Add(-25 * time.Hour),
			GracePeriod:       24 * time.Hour,
		}

		if config.IsInGracePeriod() {
			t.Error("should be past grace period")
		}
	})
}

func TestGetRotationAge(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Hour)
	config := &RotationConfig{
		RotationStartTime: startTime,
	}

	age := config.GetRotationAge()

	// Allow some tolerance for test execution time
	if age < 2*time.Hour || age > 2*time.Hour+time.Second {
		t.Errorf("expected age ~2h, got %v", age)
	}
}

func TestGetSecretCount(t *testing.T) {
	tests := []struct {
		name          string
		previousCount int
		expectedTotal int
	}{
		{"current only", 0, 1},
		{"current + 1 previous", 1, 2},
		{"current + 3 previous", 3, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RotationConfig{
				CurrentSecret:   make([]byte, 32),
				PreviousSecrets: make([][]byte, tt.previousCount),
			}

			count := config.GetSecretCount()
			if count != tt.expectedTotal {
				t.Errorf("expected %d secrets, got %d", tt.expectedTotal, count)
			}
		})
	}
}

func TestConfigString(t *testing.T) {
	t.Run("current only", func(t *testing.T) {
		config := &RotationConfig{
			SecretVersion:     1,
			RotationStartTime: time.Now().Add(-2 * time.Hour),
			PreviousSecrets:   nil,
		}

		str := config.String()
		if str == "" {
			t.Error("String() should not be empty")
		}
	})

	t.Run("with previous secrets", func(t *testing.T) {
		config := &RotationConfig{
			SecretVersion:     3,
			RotationStartTime: time.Now().Add(-2 * time.Hour),
			PreviousSecrets:   make([][]byte, 2),
			GracePeriod:       7 * 24 * time.Hour,
		}

		str := config.String()
		if str == "" {
			t.Error("String() should not be empty")
		}
	})
}

func TestLoadRotationConfigFallbackToShared(t *testing.T) {
	secret, _ := GenerateSecret()

	// Set shared secret only
	os.Setenv("OBOT_AUTH_PROVIDER_COOKIE_SECRET", secret)
	defer os.Unsetenv("OBOT_AUTH_PROVIDER_COOKIE_SECRET")

	config, err := LoadRotationConfig("entra")
	if err != nil {
		t.Errorf("should fallback to shared secret: %v", err)
	}

	if len(config.CurrentSecret) == 0 {
		t.Error("should have loaded shared secret")
	}
}

func TestLoadRotationConfigGracePeriod(t *testing.T) {
	secret, _ := GenerateSecret()

	os.Setenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET", secret)
	os.Setenv("OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD", "336h") // 14 days
	defer os.Unsetenv("OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET")
	defer os.Unsetenv("OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD")

	config, err := LoadRotationConfig("entra")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := 14 * 24 * time.Hour
	if config.GracePeriod != expected {
		t.Errorf("expected grace period %v, got %v", expected, config.GracePeriod)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
