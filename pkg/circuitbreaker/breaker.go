package circuitbreaker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/obot-platform/obot/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker"
)

var (
	// CircuitBreakerState tracks the current state of the circuit breaker by provider
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obot_auth_circuit_breaker_state",
			Help: "Current state of the circuit breaker by provider (0=closed, 1=half-open, 2=open)",
		},
		[]string{"provider"},
	)

	// RetryAttempts tracks retry attempts by provider and outcome
	RetryAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_retry_attempts_total",
			Help: "Total number of retry attempts by provider and outcome (success/failure/exhausted)",
		},
		[]string{"provider", "outcome"},
	)

	// RetryBackoffDuration tracks the backoff duration distribution
	RetryBackoffDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "obot_auth_retry_backoff_duration_seconds",
			Help:    "Distribution of retry backoff durations in seconds by provider",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"provider"},
	)
)

// Config holds circuit breaker configuration
type Config struct {
	// Provider name for metrics
	Provider string

	// MaxRequests is the maximum number of requests allowed to pass through
	// when the CircuitBreaker is half-open. Default: 1
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for the CircuitBreaker
	// to clear the internal Counts. Default: 60s
	Interval time.Duration

	// Timeout is the period of the open state, after which the state becomes half-open.
	// Default: 60s
	Timeout time.Duration

	// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
	// If ReadyToTrip returns true, the CircuitBreaker will be placed into the open state.
	// Default: 5 consecutive failures
	Threshold uint32

	// Retry configuration
	MaxRetryAttempts  int           // Default: 3
	InitialInterval   time.Duration // Default: 1s
	MaxRetryInterval  time.Duration // Default: 30s
	BackoffMultiplier float64       // Default: 2.0
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig(provider string) *Config {
	return &Config{
		Provider:          provider,
		MaxRequests:       1,
		Interval:          60 * time.Second,
		Timeout:           60 * time.Second,
		Threshold:         5,
		MaxRetryAttempts:  3,
		InitialInterval:   1 * time.Second,
		MaxRetryInterval:  30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv(provider string) *Config {
	config := DefaultConfig(provider)

	if val := os.Getenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_THRESHOLD"); val != "" {
		if threshold, err := strconv.ParseUint(val, 10, 32); err == nil {
			config.Threshold = uint32(threshold)
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_TIMEOUT"); val != "" {
		if timeout, err := time.ParseDuration(val); err == nil {
			config.Timeout = timeout
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_RETRY_MAX_ATTEMPTS"); val != "" {
		if attempts, err := strconv.Atoi(val); err == nil {
			config.MaxRetryAttempts = attempts
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_RETRY_INITIAL_INTERVAL"); val != "" {
		if interval, err := time.ParseDuration(val); err == nil {
			config.InitialInterval = interval
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_RETRY_MAX_INTERVAL"); val != "" {
		if interval, err := time.ParseDuration(val); err == nil {
			config.MaxRetryInterval = interval
		}
	}

	return config
}

// Breaker wraps gobreaker.CircuitBreaker with retry logic and metrics
type Breaker struct {
	cb       *gobreaker.CircuitBreaker
	config   *Config
	provider string
}

// New creates a new circuit breaker with the given configuration
func New(config *Config) *Breaker {
	if config == nil {
		config = DefaultConfig("unknown")
	}

	settings := gobreaker.Settings{
		Name:        fmt.Sprintf("%s-circuit-breaker", config.Provider),
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= config.Threshold
		},
		OnStateChange: func(_ string, _ gobreaker.State, to gobreaker.State) {
			// Update metrics based on new state
			var stateValue float64
			switch to {
			case gobreaker.StateClosed:
				stateValue = 0
			case gobreaker.StateHalfOpen:
				stateValue = 1
			case gobreaker.StateOpen:
				stateValue = 2
			}
			CircuitBreakerState.WithLabelValues(config.Provider).Set(stateValue)
		},
	}

	breaker := &Breaker{
		cb:       gobreaker.NewCircuitBreaker(settings),
		config:   config,
		provider: config.Provider,
	}

	// Initialize state metric
	CircuitBreakerState.WithLabelValues(config.Provider).Set(0) // Closed state

	return breaker
}

// Execute runs the given function with circuit breaker protection and retry logic
func (b *Breaker) Execute(ctx context.Context, fn func() error) error {
	return b.ExecuteWithRetry(ctx, func() error {
		_, err := b.cb.Execute(func() (interface{}, error) {
			return nil, fn()
		})
		return err
	})
}

// ExecuteWithRetry executes the function with exponential backoff retry logic
func (b *Breaker) ExecuteWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	backoff := b.config.InitialInterval

	for attempt := 0; attempt <= b.config.MaxRetryAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// First attempt or retry after backoff
		if attempt > 0 {
			// Record backoff duration
			RetryBackoffDuration.WithLabelValues(b.provider).Observe(backoff.Seconds())

			// Wait with context cancellation support
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}

			// Calculate next backoff (exponential with cap)
			backoff = time.Duration(float64(backoff) * b.config.BackoffMultiplier)
			if backoff > b.config.MaxRetryInterval {
				backoff = b.config.MaxRetryInterval
			}
		}

		// Execute the function
		start := time.Now()
		err := fn()
		duration := time.Since(start).Seconds()

		if err == nil {
			// Success
			if attempt > 0 {
				RetryAttempts.WithLabelValues(b.provider, "success").Inc()
			}
			metrics.RecordTokenRefreshSuccess(b.provider, duration)
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			// Non-retryable error, fail immediately
			metrics.RecordTokenRefreshFailure(b.provider, duration)
			return err
		}

		// Record retry attempt
		if attempt < b.config.MaxRetryAttempts {
			RetryAttempts.WithLabelValues(b.provider, "failure").Inc()
		} else {
			// Final attempt failed
			RetryAttempts.WithLabelValues(b.provider, "exhausted").Inc()
		}

		metrics.RecordTokenRefreshFailure(b.provider, duration)
	}

	// All retries exhausted
	return fmt.Errorf("token refresh failed after %d attempts: %w", b.config.MaxRetryAttempts, lastErr)
}

// isRetryableError determines if an error should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Circuit breaker open error is not retryable
	if err == gobreaker.ErrOpenState {
		return false
	}

	// Timeout errors are retryable
	if os.IsTimeout(err) {
		return true
	}

	// Network errors are generally retryable
	// Add more specific error type checks as needed
	errMsg := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"503",
		"502",
		"504",
	}

	for _, pattern := range retryablePatterns {
		if len(errMsg) > 0 && containsIgnoreCase(errMsg, pattern) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive check
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// toLower converts ASCII string to lowercase
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// State returns the current state of the circuit breaker
func (b *Breaker) State() gobreaker.State {
	return b.cb.State()
}

// Counts returns the current counts of the circuit breaker
func (b *Breaker) Counts() gobreaker.Counts {
	return b.cb.Counts()
}
