package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsOAuthCallbackResponse(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		query    map[string]string
		expected bool
	}{
		{
			name:     "valid callback with code",
			path:     "/",
			query:    map[string]string{"code": "abc123"},
			expected: true,
		},
		{
			name:     "valid callback with error",
			path:     "/",
			query:    map[string]string{"error": "access_denied"},
			expected: true,
		},
		{
			name:     "valid callback with state",
			path:     "/",
			query:    map[string]string{"state": "xyz789"},
			expected: true,
		},
		{
			name:     "valid callback with code and state",
			path:     "/",
			query:    map[string]string{"code": "abc123", "state": "xyz789"},
			expected: true,
		},
		{
			name:     "valid callback with error and state",
			path:     "/",
			query:    map[string]string{"error": "access_denied", "state": "xyz789"},
			expected: true,
		},
		{
			name:     "valid callback with all parameters",
			path:     "/",
			query:    map[string]string{"code": "abc123", "error": "none", "state": "xyz789"},
			expected: true,
		},
		{
			name:     "root path but no OAuth parameters",
			path:     "/",
			query:    map[string]string{},
			expected: false,
		},
		{
			name:     "root path with empty code",
			path:     "/",
			query:    map[string]string{"code": ""},
			expected: false,
		},
		{
			name:     "root path with empty error",
			path:     "/",
			query:    map[string]string{"error": ""},
			expected: false,
		},
		{
			name:     "root path with empty state",
			path:     "/",
			query:    map[string]string{"state": ""},
			expected: false,
		},
		{
			name:     "non-root path with code",
			path:     "/callback",
			query:    map[string]string{"code": "abc123"},
			expected: false,
		},
		{
			name:     "non-root path with error",
			path:     "/auth",
			query:    map[string]string{"error": "access_denied"},
			expected: false,
		},
		{
			name:     "non-root path with state",
			path:     "/oauth2/callback",
			query:    map[string]string{"state": "xyz789"},
			expected: false,
		},
		{
			name:     "root path with other query params",
			path:     "/",
			query:    map[string]string{"foo": "bar", "baz": "qux"},
			expected: false,
		},
		{
			name:     "empty path with code",
			path:     "",
			query:    map[string]string{"code": "abc123"},
			expected: false,
		},
		{
			name:     "nested path with code",
			path:     "/api/oauth/callback",
			query:    map[string]string{"code": "abc123"},
			expected: false,
		},
		{
			name:     "root path with whitespace-only code",
			path:     "/",
			query:    map[string]string{"code": " "},
			expected: true,
		},
		{
			name:     "root path with whitespace-only error",
			path:     "/",
			query:    map[string]string{"error": " "},
			expected: true,
		},
		{
			name:     "root path with whitespace-only state",
			path:     "/",
			query:    map[string]string{"state": " "},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+tt.path, nil)
			q := req.URL.Query()
			for k, v := range tt.query {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			result := IsOAuthCallbackResponse(req)
			if result != tt.expected {
				t.Errorf("IsOAuthCallbackResponse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHandleOAuthRedirect(t *testing.T) {
	tests := []struct {
		name               string
		path               string
		query              map[string]string
		expectedHandled    bool
		expectedStatusCode int
		expectedLocation   string
	}{
		{
			name:               "redirects with code parameter",
			path:               "/",
			query:              map[string]string{"code": "abc123"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?code=abc123",
		},
		{
			name:               "redirects with error parameter",
			path:               "/",
			query:              map[string]string{"error": "access_denied"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?error=access_denied",
		},
		{
			name:               "redirects with state parameter",
			path:               "/",
			query:              map[string]string{"state": "xyz789"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?state=xyz789",
		},
		{
			name:               "redirects with code and state",
			path:               "/",
			query:              map[string]string{"code": "abc123", "state": "xyz789"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?code=abc123&state=xyz789",
		},
		{
			name:               "redirects with error and state",
			path:               "/",
			query:              map[string]string{"error": "access_denied", "state": "xyz789"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?error=access_denied&state=xyz789",
		},
		{
			name:               "redirects with all parameters",
			path:               "/",
			query:              map[string]string{"code": "abc123", "error": "none", "state": "xyz789"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?code=abc123&error=none&state=xyz789",
		},
		{
			name:               "does not handle root path without OAuth params",
			path:               "/",
			query:              map[string]string{},
			expectedHandled:    false,
			expectedStatusCode: 0,
			expectedLocation:   "",
		},
		{
			name:               "does not handle non-root path with code",
			path:               "/callback",
			query:              map[string]string{"code": "abc123"},
			expectedHandled:    false,
			expectedStatusCode: 0,
			expectedLocation:   "",
		},
		{
			name:               "does not handle root path with empty OAuth params",
			path:               "/",
			query:              map[string]string{"code": "", "error": "", "state": ""},
			expectedHandled:    false,
			expectedStatusCode: 0,
			expectedLocation:   "",
		},
		{
			name:               "preserves additional query parameters",
			path:               "/",
			query:              map[string]string{"code": "abc123", "foo": "bar", "baz": "qux"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?baz=qux&code=abc123&foo=bar",
		},
		{
			name:               "handles special characters in query params",
			path:               "/",
			query:              map[string]string{"code": "abc+123", "state": "xyz/789="},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?code=abc%2B123&state=xyz%2F789%3D",
		},
		{
			name:               "handles whitespace in parameters",
			path:               "/",
			query:              map[string]string{"code": " abc 123 ", "state": "xyz 789"},
			expectedHandled:    true,
			expectedStatusCode: http.StatusFound,
			expectedLocation:   "http://example.com/oauth2/callback?code=+abc+123+&state=xyz+789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+tt.path, nil)
			q := req.URL.Query()
			for k, v := range tt.query {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			handled := HandleOAuthRedirect(w, req)

			if handled != tt.expectedHandled {
				t.Errorf("HandleOAuthRedirect() handled = %v, want %v", handled, tt.expectedHandled)
			}

			if tt.expectedHandled {
				if w.Code != tt.expectedStatusCode {
					t.Errorf("status code = %v, want %v", w.Code, tt.expectedStatusCode)
				}

				location := w.Header().Get("Location")
				if location != tt.expectedLocation {
					t.Errorf("Location header = %q, want %q", location, tt.expectedLocation)
				}
			} else {
				// Should not set any response when not handled
				if w.Code != 0 && w.Code != http.StatusOK {
					t.Errorf("expected no status code when not handled, got %v", w.Code)
				}
			}
		})
	}
}

func TestHandleOAuthRedirect_PreservesHost(t *testing.T) {
	tests := []struct {
		name             string
		requestURL       string
		expectedLocation string
	}{
		{
			name:             "preserves scheme and host",
			requestURL:       "https://auth.example.com/?code=abc123",
			expectedLocation: "https://auth.example.com/oauth2/callback?code=abc123",
		},
		{
			name:             "preserves different host",
			requestURL:       "http://localhost:8080/?state=xyz789",
			expectedLocation: "http://localhost:8080/oauth2/callback?state=xyz789",
		},
		{
			name:             "preserves complex URL",
			requestURL:       "https://subdomain.example.com:9443/?code=abc&state=xyz",
			expectedLocation: "https://subdomain.example.com:9443/oauth2/callback?code=abc&state=xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)
			w := httptest.NewRecorder()

			handled := HandleOAuthRedirect(w, req)
			if !handled {
				t.Fatal("expected request to be handled")
			}

			location := w.Header().Get("Location")
			if location != tt.expectedLocation {
				t.Errorf("Location header = %q, want %q", location, tt.expectedLocation)
			}
		})
	}
}

func TestHandleOAuthRedirect_Integration(t *testing.T) {
	// Test the complete flow: IsOAuthCallbackResponse → HandleOAuthRedirect
	tests := []struct {
		name        string
		requestURL  string
		shouldCheck bool
		shouldRedir bool
	}{
		{
			name:        "OAuth callback is detected and redirected",
			requestURL:  "http://example.com/?code=abc123",
			shouldCheck: true,
			shouldRedir: true,
		},
		{
			name:        "Non-OAuth request is not detected or redirected",
			requestURL:  "http://example.com/api/users",
			shouldCheck: false,
			shouldRedir: false,
		},
		{
			name:        "Root with no params is not detected or redirected",
			requestURL:  "http://example.com/",
			shouldCheck: false,
			shouldRedir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)

			isCallback := IsOAuthCallbackResponse(req)
			if isCallback != tt.shouldCheck {
				t.Errorf("IsOAuthCallbackResponse() = %v, want %v", isCallback, tt.shouldCheck)
			}

			w := httptest.NewRecorder()
			handled := HandleOAuthRedirect(w, req)
			if handled != tt.shouldRedir {
				t.Errorf("HandleOAuthRedirect() = %v, want %v", handled, tt.shouldRedir)
			}

			// Verify consistency: if IsOAuthCallbackResponse returns true,
			// HandleOAuthRedirect should also return true
			if isCallback && !handled {
				t.Error("IsOAuthCallbackResponse returned true but HandleOAuthRedirect returned false")
			}
		})
	}
}
