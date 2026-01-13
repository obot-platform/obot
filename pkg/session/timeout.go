package session

import (
	"fmt"
	"os"
	"time"
)

// TimeoutConfig holds session timeout configuration
type TimeoutConfig struct {
	// IdleTimeout is the duration after which an idle session expires
	// A session is considered idle if there has been no user activity
	// Default: 30 minutes
	IdleTimeout time.Duration

	// AbsoluteTimeout is the maximum duration a session can last regardless of activity
	// This provides a hard limit on session lifetime for security compliance
	// Default: 24 hours
	AbsoluteTimeout time.Duration

	// EnableIdleTimeout enables idle timeout tracking
	// Default: true
	EnableIdleTimeout bool

	// EnableAbsoluteTimeout enables absolute timeout enforcement
	// Default: true
	EnableAbsoluteTimeout bool
}

// DefaultTimeoutConfig returns a TimeoutConfig with secure defaults
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		IdleTimeout:           30 * time.Minute,
		AbsoluteTimeout:       24 * time.Hour,
		EnableIdleTimeout:     true,
		EnableAbsoluteTimeout: true,
	}
}

// LoadFromEnv loads timeout configuration from environment variables
func LoadFromEnv() (*TimeoutConfig, error) {
	config := DefaultTimeoutConfig()

	// Load idle timeout
	if val := os.Getenv("OBOT_AUTH_PROVIDER_SESSION_IDLE_TIMEOUT"); val != "" {
		if val == "0" || val == "disabled" || val == "false" {
			config.EnableIdleTimeout = false
		} else {
			duration, err := time.ParseDuration(val)
			if err != nil {
				return nil, fmt.Errorf("invalid OBOT_AUTH_PROVIDER_SESSION_IDLE_TIMEOUT: %w", err)
			}
			if duration < 0 {
				return nil, fmt.Errorf("OBOT_AUTH_PROVIDER_SESSION_IDLE_TIMEOUT must be non-negative")
			}
			config.IdleTimeout = duration
			config.EnableIdleTimeout = true
		}
	}

	// Load absolute timeout
	if val := os.Getenv("OBOT_AUTH_PROVIDER_SESSION_ABSOLUTE_TIMEOUT"); val != "" {
		if val == "0" || val == "disabled" || val == "false" {
			config.EnableAbsoluteTimeout = false
		} else {
			duration, err := time.ParseDuration(val)
			if err != nil {
				return nil, fmt.Errorf("invalid OBOT_AUTH_PROVIDER_SESSION_ABSOLUTE_TIMEOUT: %w", err)
			}
			if duration < 0 {
				return nil, fmt.Errorf("OBOT_AUTH_PROVIDER_SESSION_ABSOLUTE_TIMEOUT must be non-negative")
			}
			config.AbsoluteTimeout = duration
			config.EnableAbsoluteTimeout = true
		}
	}

	return config, nil
}

// Validate checks if the timeout configuration is valid
func (c *TimeoutConfig) Validate() error {
	if c.EnableIdleTimeout && c.IdleTimeout <= 0 {
		return fmt.Errorf("idle timeout must be positive when enabled")
	}

	if c.EnableAbsoluteTimeout && c.AbsoluteTimeout <= 0 {
		return fmt.Errorf("absolute timeout must be positive when enabled")
	}

	if c.EnableIdleTimeout && c.EnableAbsoluteTimeout {
		if c.IdleTimeout > c.AbsoluteTimeout {
			return fmt.Errorf("idle timeout (%v) cannot exceed absolute timeout (%v)", c.IdleTimeout, c.AbsoluteTimeout)
		}
	}

	return nil
}

// State tracks session timing information
type State struct {
	// CreatedAt is when the session was initially created
	CreatedAt time.Time

	// LastActivityAt is when the session last had user activity
	LastActivityAt time.Time

	// ExpiresAt is when the session will expire (computed)
	ExpiresAt time.Time
}

// NewState creates a new session state with current timestamps
func NewState() *State {
	now := time.Now()
	return &State{
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now,
	}
}

// UpdateActivity updates the last activity timestamp
func (s *State) UpdateActivity() {
	s.LastActivityAt = time.Now()
}

// IsExpired checks if the session has expired based on the timeout config
func (s *State) IsExpired(config *TimeoutConfig) bool {
	now := time.Now()

	// Check absolute timeout
	if config.EnableAbsoluteTimeout {
		absoluteExpiry := s.CreatedAt.Add(config.AbsoluteTimeout)
		if now.After(absoluteExpiry) {
			return true
		}
	}

	// Check idle timeout
	if config.EnableIdleTimeout {
		idleExpiry := s.LastActivityAt.Add(config.IdleTimeout)
		if now.After(idleExpiry) {
			return true
		}
	}

	return false
}

// ComputeExpiry computes when the session will expire based on the timeout config
func (s *State) ComputeExpiry(config *TimeoutConfig) time.Time {
	var expiry time.Time

	// Compute absolute expiry
	if config.EnableAbsoluteTimeout {
		absoluteExpiry := s.CreatedAt.Add(config.AbsoluteTimeout)
		if expiry.IsZero() || absoluteExpiry.Before(expiry) {
			expiry = absoluteExpiry
		}
	}

	// Compute idle expiry
	if config.EnableIdleTimeout {
		idleExpiry := s.LastActivityAt.Add(config.IdleTimeout)
		if expiry.IsZero() || idleExpiry.Before(expiry) {
			expiry = idleExpiry
		}
	}

	// If no timeouts enabled, return zero time (never expires)
	return expiry
}

// TimeUntilExpiry returns the duration until the session expires
func (s *State) TimeUntilExpiry(config *TimeoutConfig) time.Duration {
	expiry := s.ComputeExpiry(config)
	if expiry.IsZero() {
		return 0 // Never expires
	}

	remaining := time.Until(expiry)
	if remaining < 0 {
		return 0 // Already expired
	}

	return remaining
}

// ExpiryReason returns a human-readable reason for session expiry
func (s *State) ExpiryReason(config *TimeoutConfig) string {
	now := time.Now()

	// Check absolute timeout first (takes precedence in logging)
	if config.EnableAbsoluteTimeout {
		absoluteExpiry := s.CreatedAt.Add(config.AbsoluteTimeout)
		if now.After(absoluteExpiry) {
			return fmt.Sprintf("session exceeded absolute timeout of %v", config.AbsoluteTimeout)
		}
	}

	// Check idle timeout
	if config.EnableIdleTimeout {
		idleExpiry := s.LastActivityAt.Add(config.IdleTimeout)
		if now.After(idleExpiry) {
			idleDuration := now.Sub(s.LastActivityAt)
			return fmt.Sprintf("session idle for %v (idle timeout: %v)", idleDuration.Round(time.Second), config.IdleTimeout)
		}
	}

	return "session not expired"
}

// String returns a human-readable representation of the timeout config
func (c *TimeoutConfig) String() string {
	if !c.EnableIdleTimeout && !c.EnableAbsoluteTimeout {
		return "session timeouts disabled"
	}

	result := "session timeouts: "
	parts := []string{}

	if c.EnableIdleTimeout {
		parts = append(parts, fmt.Sprintf("idle=%v", c.IdleTimeout))
	}

	if c.EnableAbsoluteTimeout {
		parts = append(parts, fmt.Sprintf("absolute=%v", c.AbsoluteTimeout))
	}

	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}

	return result
}
