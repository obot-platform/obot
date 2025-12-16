// Package ratelimit provides HTTP client utilities with exponential backoff retry
// for handling rate limiting and transient errors from external APIs.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RetryConfig configures retry behavior for HTTP requests
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialBackoff time.Duration // Initial backoff duration before first retry
	MaxBackoff     time.Duration // Maximum backoff duration
	BackoffFactor  float64       // Multiplier applied to backoff after each retry
}

// DefaultConfig returns sensible defaults for API rate limiting
// These values are tuned for Microsoft Graph API and similar services
func DefaultConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
}

// DoWithRetry executes an HTTP request with exponential backoff retry on rate limiting
// It handles:
// - 429 Too Many Requests: Uses Retry-After header if present, otherwise exponential backoff
// - Transient network errors: Retries with exponential backoff
// - Context cancellation: Respects context and stops retrying
//
// Non-retriable errors (4xx other than 429, 5xx) are returned immediately.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, cfg RetryConfig) (*http.Response, error) {
	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Clone request for retry (body needs special handling if present)
		reqClone := req.Clone(ctx)
		resp, err := client.Do(reqClone)
		if err != nil {
			lastErr = err
			backoff = nextBackoff(backoff, cfg)
			continue
		}

		// Success - any 2xx response
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// Rate limited (429) - retry with backoff
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			// Use Retry-After header if present (Microsoft Graph provides this)
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					backoff = time.Duration(seconds) * time.Second
				}
			}
			lastErr = fmt.Errorf("rate limited (429)")
			backoff = nextBackoff(backoff, cfg)
			continue
		}

		// Service unavailable (503) - retry with backoff
		if resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			lastErr = fmt.Errorf("service unavailable (503)")
			backoff = nextBackoff(backoff, cfg)
			continue
		}

		// Gateway timeout (504) - retry with backoff
		if resp.StatusCode == http.StatusGatewayTimeout {
			resp.Body.Close()
			lastErr = fmt.Errorf("gateway timeout (504)")
			backoff = nextBackoff(backoff, cfg)
			continue
		}

		// Other errors - return immediately without retry
		return resp, nil
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}

// nextBackoff calculates the next backoff duration using exponential growth
func nextBackoff(current time.Duration, cfg RetryConfig) time.Duration {
	next := time.Duration(float64(current) * cfg.BackoffFactor)
	if next > cfg.MaxBackoff {
		return cfg.MaxBackoff
	}
	return next
}
