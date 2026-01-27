package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestClient_doRequest_HeaderValidation(t *testing.T) {
	t.Run("Odd number of header key-values returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{BaseURL: server.URL, Token: "test-token"}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil, "Key1", "Value1", "Key2")

		if err == nil {
			t.Error("Expected error for odd number of header key-values, got nil")
		}
		if !strings.Contains(err.Error(), "must be even") {
			t.Errorf("Expected error about even number of headers, got: %v", err)
		}
	})

	t.Run("Even number of header key-values succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify headers were set
			if r.Header.Get("X-Custom-1") != "value1" {
				t.Errorf("Expected header X-Custom-1=value1, got %s", r.Header.Get("X-Custom-1"))
			}
			if r.Header.Get("X-Custom-2") != "value2" {
				t.Errorf("Expected header X-Custom-2=value2, got %s", r.Header.Get("X-Custom-2"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{BaseURL: server.URL, Token: "test-token"}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil,
			"X-Custom-1", "value1", "X-Custom-2", "value2")

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

func TestClient_doRequest_Authentication(t *testing.T) {
	t.Run("Bearer token set in Authorization header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token-123" {
				t.Errorf("Expected Authorization header 'Bearer test-token-123', got %q", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{BaseURL: server.URL, Token: "test-token-123"}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("Cookie added to request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				t.Errorf("Expected session cookie, got error: %v", err)
			}
			if cookie.Value != "abc123" {
				t.Errorf("Expected cookie value 'abc123', got %q", cookie.Value)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{
			BaseURL: server.URL,
			Token: "test-token",
			Cookie: &http.Cookie{Name: "session", Value: "abc123"},
		}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("Token fetcher called when token not set", func(t *testing.T) {
		fetcherCalled := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer fetched-token" {
				t.Errorf("Expected Authorization header 'Bearer fetched-token', got %q", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{
			BaseURL: server.URL,
			tokenFetcher: func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) {
				fetcherCalled = true
				return "fetched-token", nil
			},
		}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !fetcherCalled {
			t.Error("Expected token fetcher to be called")
		}
		// Verify token was cached
		if client.Token != "fetched-token" {
			t.Errorf("Expected token to be cached as 'fetched-token', got %q", client.Token)
		}
	})

	t.Run("Token fetcher error propagated", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		expectedErr := fmt.Errorf("token fetch failed")
		client := &Client{
			BaseURL: server.URL,
			tokenFetcher: func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) {
				return "", expectedErr
			},
		}
		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)

		if err == nil {
			t.Error("Expected error from token fetcher, got nil")
		}
		if !strings.Contains(err.Error(), "failed to fetch token") {
			t.Errorf("Expected error about token fetch failure, got: %v", err)
		}
	})
}

func TestClient_doRequest_HTTPErrors(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "400 Bad Request",
			statusCode:     http.StatusBadRequest,
			responseBody:   "Invalid input",
			expectedErrMsg: "Invalid input",
		},
		{
			name:           "401 Unauthorized",
			statusCode:     http.StatusUnauthorized,
			responseBody:   "Authentication required",
			expectedErrMsg: "Authentication required",
		},
		{
			name:           "403 Forbidden",
			statusCode:     http.StatusForbidden,
			responseBody:   "Access denied",
			expectedErrMsg: "Access denied",
		},
		{
			name:           "404 Not Found",
			statusCode:     http.StatusNotFound,
			responseBody:   "Resource not found",
			expectedErrMsg: "Resource not found",
		},
		{
			name:           "500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "Server error",
			expectedErrMsg: "Server error",
		},
		{
			name:           "Empty error message uses status text",
			statusCode:     http.StatusBadRequest,
			responseBody:   "",
			expectedErrMsg: "400 Bad Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := &Client{BaseURL: server.URL, Token: "test-token"}
			_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)

			if err == nil {
				t.Error("Expected error for HTTP error status, got nil")
			}

			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Errorf("Expected ErrHTTP error type, got %T", err)
			}
			if httpErr.Code != tt.statusCode {
				t.Errorf("Expected status code %d, got %d", tt.statusCode, httpErr.Code)
			}
			if !strings.Contains(httpErr.Message, tt.expectedErrMsg) {
				t.Errorf("Expected error message containing %q, got %q", tt.expectedErrMsg, httpErr.Message)
			}
		})
	}
}

func TestClient_doRequest_BinaryDataHandling(t *testing.T) {
	t.Run("Binary request body handled correctly", func(t *testing.T) {
		binaryData := []byte{0xFF, 0xFE, 0xFD, 0xFC, 0x00, 0x01, 0x02}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Just verify we can receive binary data
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{BaseURL: server.URL, Token: "test-token"}
		_, _, err := client.doRequest(context.Background(), http.MethodPost, "/test",
			strings.NewReader(string(binaryData)))

		if err != nil {
			t.Errorf("Unexpected error handling binary data: %v", err)
		}
	})

	t.Run("Invalid UTF-8 in request body", func(t *testing.T) {
		invalidUTF8 := []byte{0xFF, 0xFE}
		if utf8.Valid(invalidUTF8) {
			t.Skip("Test data is actually valid UTF-8")
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &Client{BaseURL: server.URL, Token: "test-token"}
		_, _, err := client.doRequest(context.Background(), http.MethodPost, "/test",
			strings.NewReader(string(invalidUTF8)))

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}
