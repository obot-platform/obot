package circuitbreaker

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test-provider")

	if config.Provider != "test-provider" {
		t.Errorf("expected provider 'test-provider', got '%s'", config.Provider)
	}

	if config.MaxRequests != 1 {
		t.Errorf("expected MaxRequests 1, got %d", config.MaxRequests)
	}

	if config.Interval != 60*time.Second {
		t.Errorf("expected Interval 60s, got %v", config.Interval)
	}

	if config.Timeout != 60*time.Second {
		t.Errorf("expected Timeout 60s, got %v", config.Timeout)
	}

	if config.Threshold != 5 {
		t.Errorf("expected Threshold 5, got %d", config.Threshold)
	}

	if config.MaxRetryAttempts != 3 {
		t.Errorf("expected MaxRetryAttempts 3, got %d", config.MaxRetryAttempts)
	}

	if config.InitialInterval != 1*time.Second {
		t.Errorf("expected InitialInterval 1s, got %v", config.InitialInterval)
	}

	if config.MaxRetryInterval != 30*time.Second {
		t.Errorf("expected MaxRetryInterval 30s, got %v", config.MaxRetryInterval)
	}

	if config.BackoffMultiplier != 2.0 {
		t.Errorf("expected BackoffMultiplier 2.0, got %f", config.BackoffMultiplier)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_THRESHOLD", "10")
	os.Setenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_TIMEOUT", "30s")
	os.Setenv("OBOT_AUTH_PROVIDER_RETRY_MAX_ATTEMPTS", "5")
	os.Setenv("OBOT_AUTH_PROVIDER_RETRY_INITIAL_INTERVAL", "500ms")
	os.Setenv("OBOT_AUTH_PROVIDER_RETRY_MAX_INTERVAL", "60s")
	defer func() {
		os.Unsetenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_THRESHOLD")
		os.Unsetenv("OBOT_AUTH_PROVIDER_CIRCUIT_BREAKER_TIMEOUT")
		os.Unsetenv("OBOT_AUTH_PROVIDER_RETRY_MAX_ATTEMPTS")
		os.Unsetenv("OBOT_AUTH_PROVIDER_RETRY_INITIAL_INTERVAL")
		os.Unsetenv("OBOT_AUTH_PROVIDER_RETRY_MAX_INTERVAL")
	}()

	config := LoadFromEnv("test-provider")

	if config.Threshold != 10 {
		t.Errorf("expected Threshold 10, got %d", config.Threshold)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", config.Timeout)
	}

	if config.MaxRetryAttempts != 5 {
		t.Errorf("expected MaxRetryAttempts 5, got %d", config.MaxRetryAttempts)
	}

	if config.InitialInterval != 500*time.Millisecond {
		t.Errorf("expected InitialInterval 500ms, got %v", config.InitialInterval)
	}

	if config.MaxRetryInterval != 60*time.Second {
		t.Errorf("expected MaxRetryInterval 60s, got %v", config.MaxRetryInterval)
	}
}

func TestNewBreaker(t *testing.T) {
	config := &Config{
		Provider:    "test-provider",
		MaxRequests: 1,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		Threshold:   3,
	}

	breaker := New(config)

	if breaker == nil {
		t.Fatal("expected breaker to be non-nil")
	}

	if breaker.State() != gobreaker.StateClosed {
		t.Errorf("expected initial state to be Closed, got %v", breaker.State())
	}

	if breaker.provider != "test-provider" {
		t.Errorf("expected provider 'test-provider', got '%s'", breaker.provider)
	}
}

func TestExecuteSuccess(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 0 // No retries for this test
	breaker := New(config)

	ctx := context.Background()
	executed := false

	err := breaker.Execute(ctx, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !executed {
		t.Error("expected function to be executed")
	}

	if breaker.State() != gobreaker.StateClosed {
		t.Errorf("expected state to remain Closed, got %v", breaker.State())
	}
}

func TestExecuteFailureWithRetry(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 2
	config.InitialInterval = 10 * time.Millisecond
	config.MaxRetryInterval = 50 * time.Millisecond
	config.Threshold = 10 // High threshold to avoid opening circuit
	breaker := New(config)

	ctx := context.Background()
	attemptCount := 0
	retryableErr := errors.New("connection refused")

	err := breaker.Execute(ctx, func() error {
		attemptCount++
		if attemptCount < 3 {
			return retryableErr
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success after retries, got %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", attemptCount)
	}
}

func TestExecuteRetriesExhausted(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 2
	config.InitialInterval = 5 * time.Millisecond
	config.MaxRetryInterval = 20 * time.Millisecond
	config.Threshold = 10 // High threshold to avoid opening circuit
	breaker := New(config)

	ctx := context.Background()
	attemptCount := 0
	retryableErr := errors.New("timeout")

	err := breaker.Execute(ctx, func() error {
		attemptCount++
		return retryableErr
	})

	if err == nil {
		t.Error("expected error after retries exhausted")
	}

	// Should attempt: 1 initial + 2 retries = 3 total
	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	config := &Config{
		Provider:         "test-provider",
		MaxRequests:      1,
		Interval:         1 * time.Second,
		Timeout:          100 * time.Millisecond,
		Threshold:        3,
		MaxRetryAttempts: 0, // No retries for this test
	}
	breaker := New(config)

	ctx := context.Background()
	failErr := errors.New("non-retryable error")

	// Trigger threshold failures to open circuit
	for i := 0; i < 3; i++ {
		_ = breaker.Execute(ctx, func() error {
			return failErr
		})
	}

	// Circuit should now be open
	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected state to be Open after %d failures, got %v", config.Threshold, breaker.State())
	}

	// Attempt to execute - should fail immediately with ErrOpenState
	err := breaker.Execute(ctx, func() error {
		t.Error("function should not execute when circuit is open")
		return nil
	})

	if err != gobreaker.ErrOpenState {
		t.Errorf("expected ErrOpenState, got %v", err)
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	config := &Config{
		Provider:         "test-provider",
		MaxRequests:      1,
		Interval:         1 * time.Second,
		Timeout:          50 * time.Millisecond, // Short timeout for test
		Threshold:        2,
		MaxRetryAttempts: 0,
	}
	breaker := New(config)

	ctx := context.Background()
	failErr := errors.New("non-retryable error")

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = breaker.Execute(ctx, func() error {
			return failErr
		})
	}

	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected state to be Open, got %v", breaker.State())
	}

	// Wait for timeout to transition to half-open
	time.Sleep(60 * time.Millisecond)

	// Next request should transition to half-open and succeed
	err := breaker.Execute(ctx, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected success in half-open state, got %v", err)
	}

	// Should transition back to closed after success
	if breaker.State() != gobreaker.StateClosed {
		t.Errorf("expected state to be Closed after success, got %v", breaker.State())
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"timeout", errors.New("request timeout"), true},
		{"503 service unavailable", errors.New("HTTP 503 Service Unavailable"), true},
		{"502 bad gateway", errors.New("HTTP 502 Bad Gateway"), true},
		{"504 gateway timeout", errors.New("HTTP 504 Gateway Timeout"), true},
		{"temporary failure", errors.New("temporary failure in name resolution"), true},
		{"circuit breaker open", gobreaker.ErrOpenState, false},
		{"non-retryable error", errors.New("invalid credentials"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 5
	config.InitialInterval = 100 * time.Millisecond
	breaker := New(config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := breaker.Execute(ctx, func() error {
		return errors.New("timeout") // Retryable error
	})

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestExponentialBackoff(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 3
	config.InitialInterval = 10 * time.Millisecond
	config.MaxRetryInterval = 100 * time.Millisecond
	config.BackoffMultiplier = 2.0
	config.Threshold = 10 // High threshold to avoid opening circuit
	breaker := New(config)

	ctx := context.Background()
	attemptTimes := []time.Time{}

	err := breaker.Execute(ctx, func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return errors.New("timeout") // Always fail
	})

	if err == nil {
		t.Error("expected error after retries exhausted")
	}

	// Verify exponential backoff timing
	if len(attemptTimes) != 4 { // 1 initial + 3 retries
		t.Errorf("expected 4 attempts, got %d", len(attemptTimes))
	}

	// Check that delays increase exponentially (with some tolerance)
	expectedDelays := []time.Duration{
		0,                     // First attempt (no delay)
		10 * time.Millisecond, // First retry: 10ms
		20 * time.Millisecond, // Second retry: 20ms
		40 * time.Millisecond, // Third retry: 40ms
	}

	tolerance := 15 * time.Millisecond // Allow 15ms tolerance for timing
	for i := 1; i < len(attemptTimes); i++ {
		actualDelay := attemptTimes[i].Sub(attemptTimes[i-1])
		expectedDelay := expectedDelays[i]

		if actualDelay < expectedDelay-tolerance || actualDelay > expectedDelay+tolerance {
			t.Errorf("attempt %d: expected delay ~%v, got %v", i, expectedDelay, actualDelay)
		}
	}
}

func TestBackoffMaxCap(t *testing.T) {
	config := DefaultConfig("test-provider")
	config.MaxRetryAttempts = 5
	config.InitialInterval = 100 * time.Millisecond
	config.MaxRetryInterval = 200 * time.Millisecond
	config.BackoffMultiplier = 2.0
	config.Threshold = 10
	breaker := New(config)

	ctx := context.Background()
	attemptTimes := []time.Time{}

	_ = breaker.Execute(ctx, func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return errors.New("timeout")
	})

	// Verify that backoff is capped at MaxRetryInterval
	// Expected: 0, 100ms, 200ms (capped), 200ms (capped), 200ms (capped)
	for i := 3; i < len(attemptTimes); i++ {
		actualDelay := attemptTimes[i].Sub(attemptTimes[i-1])
		maxDelay := config.MaxRetryInterval + 50*time.Millisecond // Tolerance

		if actualDelay > maxDelay {
			t.Errorf("attempt %d: delay %v exceeded max %v", i, actualDelay, config.MaxRetryInterval)
		}
	}
}
