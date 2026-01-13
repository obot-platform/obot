package session

import (
	"os"
	"testing"
	"time"
)

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.IdleTimeout != 30*time.Minute {
		t.Errorf("expected idle timeout 30m, got %v", config.IdleTimeout)
	}

	if config.AbsoluteTimeout != 24*time.Hour {
		t.Errorf("expected absolute timeout 24h, got %v", config.AbsoluteTimeout)
	}

	if !config.EnableIdleTimeout {
		t.Error("expected idle timeout to be enabled by default")
	}

	if !config.EnableAbsoluteTimeout {
		t.Error("expected absolute timeout to be enabled by default")
	}
}

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name               string
		idleTimeout        string
		absoluteTimeout    string
		expectedIdle       time.Duration
		expectedAbsolute   time.Duration
		expectedIdleEn     bool
		expectedAbsoluteEn bool
		expectError        bool
	}{
		{
			name:               "defaults",
			idleTimeout:        "",
			absoluteTimeout:    "",
			expectedIdle:       30 * time.Minute,
			expectedAbsolute:   24 * time.Hour,
			expectedIdleEn:     true,
			expectedAbsoluteEn: true,
			expectError:        false,
		},
		{
			name:               "custom timeouts",
			idleTimeout:        "15m",
			absoluteTimeout:    "12h",
			expectedIdle:       15 * time.Minute,
			expectedAbsolute:   12 * time.Hour,
			expectedIdleEn:     true,
			expectedAbsoluteEn: true,
			expectError:        false,
		},
		{
			name:               "disabled idle timeout",
			idleTimeout:        "disabled",
			absoluteTimeout:    "24h",
			expectedIdle:       30 * time.Minute,
			expectedAbsolute:   24 * time.Hour,
			expectedIdleEn:     false,
			expectedAbsoluteEn: true,
			expectError:        false,
		},
		{
			name:               "disabled absolute timeout",
			idleTimeout:        "30m",
			absoluteTimeout:    "0",
			expectedIdle:       30 * time.Minute,
			expectedAbsolute:   24 * time.Hour,
			expectedIdleEn:     true,
			expectedAbsoluteEn: false,
			expectError:        false,
		},
		{
			name:               "both disabled",
			idleTimeout:        "false",
			absoluteTimeout:    "false",
			expectedIdle:       30 * time.Minute,
			expectedAbsolute:   24 * time.Hour,
			expectedIdleEn:     false,
			expectedAbsoluteEn: false,
			expectError:        false,
		},
		{
			name:        "invalid idle timeout format",
			idleTimeout: "invalid",
			expectError: true,
		},
		{
			name:            "invalid absolute timeout format",
			absoluteTimeout: "not-a-duration",
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.idleTimeout != "" {
				os.Setenv("OBOT_AUTH_PROVIDER_SESSION_IDLE_TIMEOUT", tt.idleTimeout)
				defer os.Unsetenv("OBOT_AUTH_PROVIDER_SESSION_IDLE_TIMEOUT")
			}
			if tt.absoluteTimeout != "" {
				os.Setenv("OBOT_AUTH_PROVIDER_SESSION_ABSOLUTE_TIMEOUT", tt.absoluteTimeout)
				defer os.Unsetenv("OBOT_AUTH_PROVIDER_SESSION_ABSOLUTE_TIMEOUT")
			}

			config, err := LoadFromEnv()

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

			if config.IdleTimeout != tt.expectedIdle {
				t.Errorf("expected idle timeout %v, got %v", tt.expectedIdle, config.IdleTimeout)
			}

			if config.AbsoluteTimeout != tt.expectedAbsolute {
				t.Errorf("expected absolute timeout %v, got %v", tt.expectedAbsolute, config.AbsoluteTimeout)
			}

			if config.EnableIdleTimeout != tt.expectedIdleEn {
				t.Errorf("expected idle timeout enabled=%v, got %v", tt.expectedIdleEn, config.EnableIdleTimeout)
			}

			if config.EnableAbsoluteTimeout != tt.expectedAbsoluteEn {
				t.Errorf("expected absolute timeout enabled=%v, got %v", tt.expectedAbsoluteEn, config.EnableAbsoluteTimeout)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *TimeoutConfig
		expectError bool
	}{
		{
			name:        "valid default config",
			config:      DefaultTimeoutConfig(),
			expectError: false,
		},
		{
			name: "valid custom config",
			config: &TimeoutConfig{
				IdleTimeout:           15 * time.Minute,
				AbsoluteTimeout:       2 * time.Hour,
				EnableIdleTimeout:     true,
				EnableAbsoluteTimeout: true,
			},
			expectError: false,
		},
		{
			name: "idle timeout exceeds absolute timeout",
			config: &TimeoutConfig{
				IdleTimeout:           2 * time.Hour,
				AbsoluteTimeout:       1 * time.Hour,
				EnableIdleTimeout:     true,
				EnableAbsoluteTimeout: true,
			},
			expectError: true,
		},
		{
			name: "negative idle timeout",
			config: &TimeoutConfig{
				IdleTimeout:       -1 * time.Minute,
				EnableIdleTimeout: true,
			},
			expectError: true,
		},
		{
			name: "negative absolute timeout",
			config: &TimeoutConfig{
				AbsoluteTimeout:       -1 * time.Hour,
				EnableAbsoluteTimeout: true,
			},
			expectError: true,
		},
		{
			name: "zero idle timeout when enabled",
			config: &TimeoutConfig{
				IdleTimeout:       0,
				EnableIdleTimeout: true,
			},
			expectError: true,
		},
		{
			name: "disabled timeouts are valid",
			config: &TimeoutConfig{
				IdleTimeout:           0,
				AbsoluteTimeout:       0,
				EnableIdleTimeout:     false,
				EnableAbsoluteTimeout: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestStateIsExpired(t *testing.T) {
	config := &TimeoutConfig{
		IdleTimeout:           5 * time.Minute,
		AbsoluteTimeout:       1 * time.Hour,
		EnableIdleTimeout:     true,
		EnableAbsoluteTimeout: true,
	}

	// Test idle timeout expiry
	t.Run("idle timeout expired", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-10 * time.Minute),
			LastActivityAt: time.Now().Add(-10 * time.Minute),
		}

		if !state.IsExpired(config) {
			t.Error("expected session to be expired due to idle timeout")
		}
	})

	// Test active session not expired
	t.Run("active session not expired", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-10 * time.Minute),
			LastActivityAt: time.Now().Add(-2 * time.Minute),
		}

		if state.IsExpired(config) {
			t.Error("expected session not to be expired")
		}
	})

	// Test absolute timeout expiry
	t.Run("absolute timeout expired", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-2 * time.Hour),
			LastActivityAt: time.Now().Add(-1 * time.Minute), // Recent activity
		}

		if !state.IsExpired(config) {
			t.Error("expected session to be expired due to absolute timeout")
		}
	})

	// Test disabled timeouts
	t.Run("no timeouts enabled", func(t *testing.T) {
		disabledConfig := &TimeoutConfig{
			EnableIdleTimeout:     false,
			EnableAbsoluteTimeout: false,
		}

		state := &State{
			CreatedAt:      time.Now().Add(-100 * time.Hour),
			LastActivityAt: time.Now().Add(-100 * time.Hour),
		}

		if state.IsExpired(disabledConfig) {
			t.Error("expected session not to be expired when timeouts disabled")
		}
	})
}

func TestStateComputeExpiry(t *testing.T) {
	config := &TimeoutConfig{
		IdleTimeout:           5 * time.Minute,
		AbsoluteTimeout:       1 * time.Hour,
		EnableIdleTimeout:     true,
		EnableAbsoluteTimeout: true,
	}

	t.Run("idle expiry sooner than absolute", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now(),
			LastActivityAt: time.Now(),
		}

		expiry := state.ComputeExpiry(config)
		expectedIdleExpiry := state.LastActivityAt.Add(config.IdleTimeout)

		// Expiry should be based on idle timeout (sooner)
		if expiry.Sub(expectedIdleExpiry).Abs() > time.Second {
			t.Errorf("expected expiry ~%v, got %v", expectedIdleExpiry, expiry)
		}
	})

	t.Run("absolute expiry sooner than idle", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-58 * time.Minute),
			LastActivityAt: time.Now(), // Recent activity
		}

		expiry := state.ComputeExpiry(config)
		expectedAbsoluteExpiry := state.CreatedAt.Add(config.AbsoluteTimeout)

		// Expiry should be based on absolute timeout (sooner)
		if expiry.Sub(expectedAbsoluteExpiry).Abs() > time.Second {
			t.Errorf("expected expiry ~%v, got %v", expectedAbsoluteExpiry, expiry)
		}
	})

	t.Run("no timeouts enabled", func(t *testing.T) {
		disabledConfig := &TimeoutConfig{
			EnableIdleTimeout:     false,
			EnableAbsoluteTimeout: false,
		}

		state := NewState()
		expiry := state.ComputeExpiry(disabledConfig)

		if !expiry.IsZero() {
			t.Error("expected zero expiry when timeouts disabled")
		}
	})
}

func TestStateTimeUntilExpiry(t *testing.T) {
	config := &TimeoutConfig{
		IdleTimeout:           5 * time.Minute,
		AbsoluteTimeout:       1 * time.Hour,
		EnableIdleTimeout:     true,
		EnableAbsoluteTimeout: true,
	}

	t.Run("time remaining", func(t *testing.T) {
		state := NewState()
		remaining := state.TimeUntilExpiry(config)

		// Should have approximately 5 minutes remaining (idle timeout)
		if remaining < 4*time.Minute || remaining > 6*time.Minute {
			t.Errorf("expected ~5m remaining, got %v", remaining)
		}
	})

	t.Run("already expired", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-2 * time.Hour),
			LastActivityAt: time.Now().Add(-2 * time.Hour),
		}

		remaining := state.TimeUntilExpiry(config)

		if remaining != 0 {
			t.Errorf("expected 0 remaining for expired session, got %v", remaining)
		}
	})

	t.Run("no timeouts enabled", func(t *testing.T) {
		disabledConfig := &TimeoutConfig{
			EnableIdleTimeout:     false,
			EnableAbsoluteTimeout: false,
		}

		state := NewState()
		remaining := state.TimeUntilExpiry(disabledConfig)

		if remaining != 0 {
			t.Errorf("expected 0 (never expires) when timeouts disabled, got %v", remaining)
		}
	})
}

func TestStateUpdateActivity(t *testing.T) {
	state := NewState()
	originalActivity := state.LastActivityAt

	time.Sleep(10 * time.Millisecond)
	state.UpdateActivity()

	if !state.LastActivityAt.After(originalActivity) {
		t.Error("expected LastActivityAt to be updated")
	}

	if state.CreatedAt != originalActivity {
		t.Error("expected CreatedAt to remain unchanged")
	}
}

func TestExpiryReason(t *testing.T) {
	config := &TimeoutConfig{
		IdleTimeout:           5 * time.Minute,
		AbsoluteTimeout:       1 * time.Hour,
		EnableIdleTimeout:     true,
		EnableAbsoluteTimeout: true,
	}

	t.Run("idle timeout reason", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-10 * time.Minute),
			LastActivityAt: time.Now().Add(-10 * time.Minute),
		}

		reason := state.ExpiryReason(config)
		if reason == "" || reason == "session not expired" {
			t.Errorf("expected idle timeout reason, got: %s", reason)
		}
	})

	t.Run("absolute timeout reason", func(t *testing.T) {
		state := &State{
			CreatedAt:      time.Now().Add(-2 * time.Hour),
			LastActivityAt: time.Now(), // Recent activity
		}

		reason := state.ExpiryReason(config)
		if reason == "" || reason == "session not expired" {
			t.Errorf("expected absolute timeout reason, got: %s", reason)
		}
	})

	t.Run("not expired", func(t *testing.T) {
		state := NewState()

		reason := state.ExpiryReason(config)
		if reason != "session not expired" {
			t.Errorf("expected 'session not expired', got: %s", reason)
		}
	})
}

func TestTimeoutConfigString(t *testing.T) {
	tests := []struct {
		name     string
		config   *TimeoutConfig
		expected string
	}{
		{
			name:     "default config",
			config:   DefaultTimeoutConfig(),
			expected: "session timeouts: idle=30m0s, absolute=24h0m0s",
		},
		{
			name: "idle only",
			config: &TimeoutConfig{
				IdleTimeout:       15 * time.Minute,
				EnableIdleTimeout: true,
			},
			expected: "session timeouts: idle=15m0s",
		},
		{
			name: "absolute only",
			config: &TimeoutConfig{
				AbsoluteTimeout:       12 * time.Hour,
				EnableAbsoluteTimeout: true,
			},
			expected: "session timeouts: absolute=12h0m0s",
		},
		{
			name: "disabled",
			config: &TimeoutConfig{
				EnableIdleTimeout:     false,
				EnableAbsoluteTimeout: false,
			},
			expected: "session timeouts disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
