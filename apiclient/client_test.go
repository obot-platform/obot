package apiclient

import (
	"context"
	"net/http"
	"os"
	"testing"
)

func TestNewClientFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		serverURL   string
		token       string
		expectedURL string
	}{
		{
			name:        "Default URL and token when env vars not set",
			serverURL:   "",
			token:       "",
			expectedURL: "http://localhost:8080/api",
		},
		{
			name:        "Custom URL without /api suffix",
			serverURL:   "http://example.com:9090",
			token:       "test-token",
			expectedURL: "http://example.com:9090/api",
		},
		{
			name:        "Custom URL with /api suffix",
			serverURL:   "http://example.com:9090/api",
			token:       "test-token",
			expectedURL: "http://example.com:9090/api",
		},
		{
			name:        "Custom URL with trailing slash",
			serverURL:   "http://example.com/",
			token:       "",
			expectedURL: "http://example.com/api",
		},
		{
			name:        "Custom URL with trailing slash and /api",
			serverURL:   "http://example.com/api/",
			token:       "",
			expectedURL: "http://example.com/api",
		},
		{
			name:        "HTTPS URL",
			serverURL:   "https://secure.example.com",
			token:       "secure-token",
			expectedURL: "https://secure.example.com/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars and restore after test
			origURL := os.Getenv("OBOT_SERVER_URL")
			origToken := os.Getenv("OBOT_TOKEN")
			defer func() {
				os.Setenv("OBOT_SERVER_URL", origURL)
				os.Setenv("OBOT_TOKEN", origToken)
			}()

			// Set test env vars
			if tt.serverURL != "" {
				os.Setenv("OBOT_SERVER_URL", tt.serverURL)
			} else {
				os.Unsetenv("OBOT_SERVER_URL")
			}
			if tt.token != "" {
				os.Setenv("OBOT_TOKEN", tt.token)
			} else {
				os.Unsetenv("OBOT_TOKEN")
			}

			client := NewClientFromEnv()

			if client.BaseURL != tt.expectedURL {
				t.Errorf("Expected BaseURL %q, got %q", tt.expectedURL, client.BaseURL)
			}
			if client.Token != tt.token {
				t.Errorf("Expected Token %q, got %q", tt.token, client.Token)
			}
		})
	}
}

func TestClient_WithMethods(t *testing.T) {
	t.Run("WithToken", func(t *testing.T) {
		original := &Client{BaseURL: "http://test", Token: "old-token"}
		newClient := original.WithToken("new-token")

		// Verify new client has new token
		if newClient.Token != "new-token" {
			t.Errorf("Expected Token %q, got %q", "new-token", newClient.Token)
		}

		// Verify original is unchanged
		if original.Token != "old-token" {
			t.Errorf("Original client modified: expected Token %q, got %q", "old-token", original.Token)
		}

		// Verify BaseURL copied
		if newClient.BaseURL != original.BaseURL {
			t.Errorf("Expected BaseURL %q, got %q", original.BaseURL, newClient.BaseURL)
		}
	})

	t.Run("WithCookie", func(t *testing.T) {
		original := &Client{BaseURL: "http://test"}
		cookie := &http.Cookie{Name: "session", Value: "abc123"}
		newClient := original.WithCookie(cookie)

		// Verify new client has cookie
		if newClient.Cookie == nil || newClient.Cookie.Value != "abc123" {
			t.Errorf("Expected cookie with value %q, got %v", "abc123", newClient.Cookie)
		}

		// Verify original is unchanged
		if original.Cookie != nil {
			t.Errorf("Original client modified")
		}
	})

	t.Run("WithTokenFetcher", func(t *testing.T) {
		original := &Client{BaseURL: "http://test"}
		fetcher := func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) {
			return "fetched-token", nil
		}
		newClient := original.WithTokenFetcher(fetcher)

		// Verify new client has token fetcher
		if newClient.tokenFetcher == nil {
			t.Error("Expected tokenFetcher to be set")
		}

		// Verify original is unchanged
		if original.tokenFetcher != nil {
			t.Errorf("Original client modified")
		}

		// Test the fetcher works
		token, err := newClient.GetToken(context.Background(), false, false)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if token != "fetched-token" {
			t.Errorf("Expected token %q, got %q", "fetched-token", token)
		}
	})
}

func TestClient_GetToken(t *testing.T) {
	tests := []struct {
		name          string
		client        *Client
		noExpiration  bool
		forceRefresh  bool
		expectedToken string
		expectError   bool
	}{
		{
			name:          "Returns existing token when set and not forcing refresh",
			client:        &Client{Token: "existing-token"},
			noExpiration:  false,
			forceRefresh:  false,
			expectedToken: "existing-token",
			expectError:   false,
		},
		{
			name:         "Calls token fetcher when forcing refresh",
			client:       &Client{Token: "existing-token", tokenFetcher: func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) { return "new-token", nil }},
			noExpiration: false,
			forceRefresh: true,
			expectedToken: "new-token",
			expectError:   false,
		},
		{
			name:         "Calls token fetcher when token not set",
			client:       &Client{tokenFetcher: func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) { return "fetched-token", nil }},
			noExpiration: false,
			forceRefresh: false,
			expectedToken: "fetched-token",
			expectError:   false,
		},
		{
			name:         "Returns error when no token or fetcher",
			client:       &Client{},
			noExpiration: false,
			forceRefresh: false,
			expectError:  true,
		},
		{
			name:         "Passes noExpiration flag to fetcher",
			client:       &Client{tokenFetcher: func(ctx context.Context, url string, noExp bool, forceRefresh bool) (string, error) {
				if noExp {
					return "no-exp-token", nil
				}
				return "exp-token", nil
			}},
			noExpiration:  true,
			forceRefresh:  false,
			expectedToken: "no-exp-token",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tt.client.GetToken(context.Background(), tt.noExpiration, tt.forceRefresh)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && token != tt.expectedToken {
				t.Errorf("Expected token %q, got %q", tt.expectedToken, token)
			}
		})
	}
}

func TestClient_runURLFromOpts(t *testing.T) {
	tests := []struct {
		name        string
		opts        ListRunsOptions
		expectedURL string
	}{
		{
			name:        "Default runs URL when no options",
			opts:        ListRunsOptions{},
			expectedURL: "/runs",
		},
		{
			name:        "Agent-specific runs URL",
			opts:        ListRunsOptions{AgentID: "agent-123"},
			expectedURL: "/agents/agent-123/runs",
		},
		{
			name:        "Thread-specific runs URL",
			opts:        ListRunsOptions{ThreadID: "thread-456"},
			expectedURL: "/threads/thread-456/runs",
		},
		{
			name:        "Agent and thread-specific runs URL",
			opts:        ListRunsOptions{AgentID: "agent-123", ThreadID: "thread-456"},
			expectedURL: "/agents/agent-123/threads/thread-456/runs",
		},
		{
			name:        "Empty agent ID with thread ID",
			opts:        ListRunsOptions{AgentID: "", ThreadID: "thread-789"},
			expectedURL: "/threads/thread-789/runs",
		},
		{
			name:        "Agent ID with empty thread ID",
			opts:        ListRunsOptions{AgentID: "agent-999", ThreadID: ""},
			expectedURL: "/agents/agent-999/runs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			url := client.runURLFromOpts(tt.opts)

			if url != tt.expectedURL {
				t.Errorf("Expected URL %q, got %q", tt.expectedURL, url)
			}
		})
	}
}
