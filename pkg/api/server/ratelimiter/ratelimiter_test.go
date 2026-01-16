package ratelimiter

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// mockUserInfo implements user.Info for testing
type mockUserInfo struct {
	uid    string
	name   string
	groups []string
}

func (m *mockUserInfo) GetName() string                { return m.name }
func (m *mockUserInfo) GetUID() string                 { return m.uid }
func (m *mockUserInfo) GetGroups() []string            { return m.groups }
func (m *mockUserInfo) GetExtra() map[string][]string  { return nil }

var _ user.Info = (*mockUserInfo)(nil) // Ensure mockUserInfo implements user.Info

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		expectError bool
		description string
	}{
		{
			name: "valid default options",
			opts: Options{
				UnauthenticatedRateLimit: 100,
				AuthenticatedRateLimit:   200,
			},
			expectError: false,
			description: "Standard rate limits should initialize successfully",
		},
		{
			name: "zero unauthenticated limit",
			opts: Options{
				UnauthenticatedRateLimit: 0,
				AuthenticatedRateLimit:   200,
			},
			expectError: false,
			description: "Zero limits are valid (effectively no rate limit)",
		},
		{
			name: "zero authenticated limit",
			opts: Options{
				UnauthenticatedRateLimit: 100,
				AuthenticatedRateLimit:   0,
			},
			expectError: false,
			description: "Zero authenticated limit is valid",
		},
		{
			name: "both limits zero",
			opts: Options{
				UnauthenticatedRateLimit: 0,
				AuthenticatedRateLimit:   0,
			},
			expectError: false,
			description: "Both zero limits should be valid",
		},
		{
			name: "high rate limits",
			opts: Options{
				UnauthenticatedRateLimit: 10000,
				AuthenticatedRateLimit:   50000,
			},
			expectError: false,
			description: "Very high rate limits should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := New(tt.opts)
			if tt.expectError && err == nil {
				t.Errorf("New() expected error but got none. Description: %s", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("New() unexpected error: %v. Description: %s", err, tt.description)
			}
			if !tt.expectError && limiter == nil {
				t.Error("New() returned nil limiter without error")
			}
		})
	}
}

func TestRateLimiter_ApplyLimit_AdminExemption(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 1, // Very restrictive
		AuthenticatedRateLimit:   1,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	adminUser := &mockUserInfo{
		uid:    "admin-123",
		name:   "admin",
		groups: []string{types.GroupAdmin, types.GroupAuthenticated},
	}

	// Admin should be able to make unlimited requests
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		rw := httptest.NewRecorder()

		err := limiter.ApplyLimit(adminUser, rw, req)
		if err != nil {
			t.Errorf("iteration %d: admin should be exempt from rate limiting, got error: %v", i, err)
		}

		// No rate limit headers should be set for admins
		if rw.Header().Get(headerRateLimitLimit) != "" {
			t.Errorf("iteration %d: admin should not have rate limit headers set", i)
		}
	}
}

func TestRateLimiter_ApplyLimit_AuthenticatedUser(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 100,
		AuthenticatedRateLimit:   5, // 5 requests per second
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	authenticatedUser := &mockUserInfo{
		uid:    "user-456",
		name:   "regular-user",
		groups: []string{types.GroupAuthenticated},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)

	// First 5 requests should succeed
	for i := 0; i < 5; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(authenticatedUser, rw, req)
		if err != nil {
			t.Errorf("request %d should succeed, got error: %v", i+1, err)
		}

		// Check rate limit headers are set
		limit := rw.Header().Get(headerRateLimitLimit)
		if limit != "5" {
			t.Errorf("request %d: expected limit header '5', got %q", i+1, limit)
		}

		remaining := rw.Header().Get(headerRateLimitRemaining)
		expectedRemaining := strconv.Itoa(4 - i)
		if remaining != expectedRemaining {
			t.Errorf("request %d: expected remaining %q, got %q", i+1, expectedRemaining, remaining)
		}
	}

	// 6th request should fail
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(authenticatedUser, rw, req)
	if err == nil {
		t.Error("6th request should be rate limited")
	}
	if !errors.Is(err, ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got: %v", err)
	}

	// Check Retry-After header is set
	retryAfter := rw.Header().Get(headerRetryAfter)
	if retryAfter == "" {
		t.Error("Retry-After header should be set when rate limited")
	}

	// Check remaining is 0
	remaining := rw.Header().Get(headerRateLimitRemaining)
	if remaining != "0" {
		t.Errorf("expected remaining '0', got %q", remaining)
	}
}

func TestRateLimiter_ApplyLimit_UnauthenticatedByIP(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 3, // 3 requests per second
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	unauthUser := &mockUserInfo{
		uid:    "",
		name:   "",
		groups: []string{}, // Not authenticated
	}

	// Test different IP addresses get separate limits
	ips := []string{"192.168.1.1:54321", "192.168.1.2:54322", "10.0.0.1:12345"}

	for _, ip := range ips {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		req.RemoteAddr = ip

		// Each IP should get 3 requests
		for i := 0; i < 3; i++ {
			rw := httptest.NewRecorder()
			err := limiter.ApplyLimit(unauthUser, rw, req)
			if err != nil {
				t.Errorf("IP %s request %d should succeed, got error: %v", ip, i+1, err)
			}
		}

		// 4th request should fail for this IP
		rw := httptest.NewRecorder()
		err = limiter.ApplyLimit(unauthUser, rw, req)
		if err == nil {
			t.Errorf("IP %s 4th request should be rate limited", ip)
		}
	}
}

func TestRateLimiter_ApplyLimit_IPPortStripping(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 2,
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	unauthUser := &mockUserInfo{
		uid:    "",
		name:   "",
		groups: []string{},
	}

	// Same IP with different ports should share the same limit
	req1 := httptest.NewRequest("GET", "http://example.com", nil)
	req1.RemoteAddr = "192.168.1.100:54321"

	req2 := httptest.NewRequest("GET", "http://example.com", nil)
	req2.RemoteAddr = "192.168.1.100:54322"

	// First request from port 54321
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req1)
	if err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}

	// Second request from different port but same IP
	rw = httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req2)
	if err != nil {
		t.Fatalf("second request should succeed: %v", err)
	}

	// Third request should be rate limited (shares limit with first two)
	rw = httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req1)
	if err == nil {
		t.Error("third request should be rate limited (port should be stripped)")
	}
}

func TestRateLimiter_ApplyLimit_AuthenticatedUserKeyPriority(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 100,
		AuthenticatedRateLimit:   2,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	// Test UID is used when present
	userWithUID := &mockUserInfo{
		uid:    "user-uid-123",
		name:   "user-name",
		groups: []string{types.GroupAuthenticated},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)

	// Use 2 requests for user with UID
	for i := 0; i < 2; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(userWithUID, rw, req)
		if err != nil {
			t.Errorf("request %d should succeed: %v", i+1, err)
		}
	}

	// 3rd should fail
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(userWithUID, rw, req)
	if err == nil {
		t.Error("3rd request should be rate limited")
	}

	// Test fallback to name when UID is empty
	userWithoutUID := &mockUserInfo{
		uid:    "",
		name:   "another-user",
		groups: []string{types.GroupAuthenticated},
	}

	// This user should have their own separate limit
	for i := 0; i < 2; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(userWithoutUID, rw, req)
		if err != nil {
			t.Errorf("user without UID request %d should succeed: %v", i+1, err)
		}
	}
}

func TestRateLimiter_ApplyLimit_UnauthenticatedFallbackWhenNoKey(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 2,
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	// User has authenticated group but no UID or name
	// Should fall back to unauthenticated (IP-based) limit
	userNoKey := &mockUserInfo{
		uid:    "",
		name:   "",
		groups: []string{types.GroupAuthenticated},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "10.0.0.5:54321"

	// Should use unauthenticated limit (2 req/sec)
	for i := 0; i < 2; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(userNoKey, rw, req)
		if err != nil {
			t.Errorf("request %d should succeed: %v", i+1, err)
		}
	}

	// 3rd should fail
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(userNoKey, rw, req)
	if err == nil {
		t.Error("3rd request should be rate limited using unauthenticated limit")
	}
}

func TestRateLimiter_ApplyLimit_HeadersFormat(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 10,
		AuthenticatedRateLimit:   10,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	user := &mockUserInfo{
		uid:    "test-user",
		name:   "test",
		groups: []string{types.GroupAuthenticated},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)
	rw := httptest.NewRecorder()

	err = limiter.ApplyLimit(user, rw, req)
	if err != nil {
		t.Fatalf("request should succeed: %v", err)
	}

	// Check X-RateLimit-Limit is numeric
	limitHeader := rw.Header().Get(headerRateLimitLimit)
	if _, err := strconv.ParseUint(limitHeader, 10, 64); err != nil {
		t.Errorf("X-RateLimit-Limit should be numeric, got: %q", limitHeader)
	}

	// Check X-RateLimit-Remaining is numeric
	remainingHeader := rw.Header().Get(headerRateLimitRemaining)
	if _, err := strconv.ParseUint(remainingHeader, 10, 64); err != nil {
		t.Errorf("X-RateLimit-Remaining should be numeric, got: %q", remainingHeader)
	}

	// Check X-RateLimit-Reset is RFC1123 format
	resetHeader := rw.Header().Get(headerRateLimitReset)
	if _, err := time.Parse(time.RFC1123, resetHeader); err != nil {
		t.Errorf("X-RateLimit-Reset should be RFC1123 format, got: %q (error: %v)", resetHeader, err)
	}
}

func TestRateLimiter_ApplyLimit_IPFromXForwardedFor(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 2,
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	unauthUser := &mockUserInfo{
		uid:    "",
		name:   "",
		groups: []string{},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.45, 198.51.100.23")

	// Should use the extracted IP from X-Forwarded-For (198.51.100.23)
	for i := 0; i < 2; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(unauthUser, rw, req)
		if err != nil {
			t.Errorf("request %d should succeed: %v", i+1, err)
		}
	}

	// 3rd request should fail
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req)
	if err == nil {
		t.Error("3rd request should be rate limited")
	}

	// Different X-Forwarded-For IP should have separate limit
	req2 := httptest.NewRequest("GET", "http://example.com", nil)
	req2.RemoteAddr = "10.0.0.1:54321"
	req2.Header.Set("X-Forwarded-For", "203.0.113.99")

	rw = httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req2)
	if err != nil {
		t.Error("different IP should have separate limit")
	}
}

func TestRateLimiter_ApplyLimit_ContextCancellation(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 10,
		AuthenticatedRateLimit:   10,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	user := &mockUserInfo{
		uid:    "test-user",
		name:   "test",
		groups: []string{types.GroupAuthenticated},
	}

	// Create a request with a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest("GET", "http://example.com", nil)
	req = req.WithContext(ctx)
	rw := httptest.NewRecorder()

	err = limiter.ApplyLimit(user, rw, req)
	// The limiter should handle context cancellation gracefully
	// It might return an error or succeed depending on timing
	// We mainly want to ensure it doesn't panic
	if err != nil && !strings.Contains(err.Error(), "context") {
		t.Logf("ApplyLimit with canceled context returned error: %v", err)
	}
}

func TestRateLimiter_ApplyLimit_IPv6Support(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 2,
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	unauthUser := &mockUserInfo{
		uid:    "",
		name:   "",
		groups: []string{},
	}

	// Test IPv6 address with port
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "[2001:db8::1]:54321"

	// Should strip port and use IPv6 address
	for i := 0; i < 2; i++ {
		rw := httptest.NewRecorder()
		err := limiter.ApplyLimit(unauthUser, rw, req)
		if err != nil {
			t.Errorf("IPv6 request %d should succeed: %v", i+1, err)
		}
	}

	// 3rd should fail
	rw := httptest.NewRecorder()
	err = limiter.ApplyLimit(unauthUser, rw, req)
	if err == nil {
		t.Error("3rd IPv6 request should be rate limited")
	}
}
