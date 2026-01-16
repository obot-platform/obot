package auth

import (
	"context"
	"testing"
	"time"
)

func TestSerializableState_ProviderUsername(t *testing.T) {
	tests := []struct {
		name         string
		state        SerializableState
		providerName string
		want         string
	}{
		{
			name: "github provider uses preferred username",
			state: SerializableState{
				PreferredUsername: "github-user-id",
				User:              "user@example.com",
				Email:             "user@example.com",
			},
			providerName: "github-auth-provider",
			want:         "github-user-id",
		},
		{
			name: "github provider with empty preferred username",
			state: SerializableState{
				PreferredUsername: "",
				User:              "user@example.com",
				Email:             "user@example.com",
			},
			providerName: "github-auth-provider",
			want:         "",
		},
		{
			name: "non-github provider uses user field",
			state: SerializableState{
				PreferredUsername: "preferred",
				User:              "john.doe",
				Email:             "john@example.com",
			},
			providerName: "google-auth-provider",
			want:         "john.doe",
		},
		{
			name: "non-github provider falls back to email when user is empty",
			state: SerializableState{
				PreferredUsername: "preferred",
				User:              "",
				Email:             "john@example.com",
			},
			providerName: "google-auth-provider",
			want:         "john@example.com",
		},
		{
			name: "non-github provider with both user and email empty",
			state: SerializableState{
				PreferredUsername: "preferred",
				User:              "",
				Email:             "",
			},
			providerName: "google-auth-provider",
			want:         "",
		},
		{
			name: "empty provider name uses user field",
			state: SerializableState{
				PreferredUsername: "preferred",
				User:              "john.doe",
				Email:             "john@example.com",
			},
			providerName: "",
			want:         "john.doe",
		},
		{
			name: "case sensitive provider name check",
			state: SerializableState{
				PreferredUsername: "github-id",
				User:              "john.doe",
				Email:             "john@example.com",
			},
			providerName: "GitHub-Auth-Provider",
			want:         "john.doe",
		},
		{
			name: "provider name with spaces not treated as github",
			state: SerializableState{
				PreferredUsername: "github-id",
				User:              "john.doe",
				Email:             "john@example.com",
			},
			providerName: "github-auth-provider ",
			want:         "john.doe",
		},
		{
			name: "all fields empty",
			state: SerializableState{
				PreferredUsername: "",
				User:              "",
				Email:             "",
			},
			providerName: "some-provider",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.ProviderUsername(tt.providerName)
			if got != tt.want {
				t.Errorf("ProviderUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContextWithProviderURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "http URL",
			url:  "http://example.com/auth",
		},
		{
			name: "https URL",
			url:  "https://auth.example.com/oauth",
		},
		{
			name: "empty URL",
			url:  "",
		},
		{
			name: "URL with path and query",
			url:  "https://example.com/auth?client_id=123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = ContextWithProviderURL(ctx, tt.url)

			got := ProviderURLFromContext(ctx)
			if got != tt.url {
				t.Errorf("ProviderURLFromContext() = %q, want %q", got, tt.url)
			}
		})
	}
}

func TestProviderURLFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "context without provider URL",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "context with provider URL",
			ctx:  ContextWithProviderURL(context.Background(), "https://example.com"),
			want: "https://example.com",
		},
		{
			name: "context with empty provider URL",
			ctx:  ContextWithProviderURL(context.Background(), ""),
			want: "",
		},
		{
			name: "nil context",
			ctx:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Handle nil context case with defer/recover to avoid panic
			if tt.ctx == nil {
				defer func() {
					if r := recover(); r == nil {
						t.Error("expected panic for nil context, but didn't panic")
					}
				}()
			}

			got := ProviderURLFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("ProviderURLFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstExtraValue(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string][]string
		key   string
		want  string
	}{
		{
			name: "single value",
			extra: map[string][]string{
				"role": {"admin"},
			},
			key:  "role",
			want: "admin",
		},
		{
			name: "multiple values returns first",
			extra: map[string][]string{
				"groups": {"admin", "users", "developers"},
			},
			key:  "groups",
			want: "admin",
		},
		{
			name: "empty slice",
			extra: map[string][]string{
				"empty": {},
			},
			key:  "empty",
			want: "",
		},
		{
			name:  "key not present",
			extra: map[string][]string{},
			key:   "missing",
			want:  "",
		},
		{
			name:  "nil map",
			extra: nil,
			key:   "any",
			want:  "",
		},
		{
			name: "empty string in slice",
			extra: map[string][]string{
				"empty-value": {""},
			},
			key:  "empty-value",
			want: "",
		},
		{
			name: "whitespace values",
			extra: map[string][]string{
				"spaces": {"  ", "value"},
			},
			key:  "spaces",
			want: "  ",
		},
		{
			name: "special characters",
			extra: map[string][]string{
				"special": {"user@example.com", "other"},
			},
			key:  "special",
			want: "user@example.com",
		},
		{
			name: "unicode values",
			extra: map[string][]string{
				"unicode": {"用户", "user"},
			},
			key:  "unicode",
			want: "用户",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirstExtraValue(tt.extra, tt.key)
			if got != tt.want {
				t.Errorf("FirstExtraValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSerializableState_CompleteStructure(t *testing.T) {
	// Test that all fields are properly set and accessible
	expiresOn := time.Now().Add(time.Hour)
	state := SerializableState{
		ExpiresOn:         &expiresOn,
		AccessToken:       "token123",
		PreferredUsername: "preferred",
		User:              "user@example.com",
		Email:             "user@example.com",
		SetCookies:        []string{"cookie1=value1", "cookie2=value2"},
	}

	// Verify all fields are set correctly
	if state.ExpiresOn == nil {
		t.Error("ExpiresOn should not be nil")
	}
	if state.AccessToken != "token123" {
		t.Errorf("AccessToken = %q, want %q", state.AccessToken, "token123")
	}
	if state.PreferredUsername != "preferred" {
		t.Errorf("PreferredUsername = %q, want %q", state.PreferredUsername, "preferred")
	}
	if state.User != "user@example.com" {
		t.Errorf("User = %q, want %q", state.User, "user@example.com")
	}
	if state.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", state.Email, "user@example.com")
	}
	if len(state.SetCookies) != 2 {
		t.Errorf("SetCookies length = %d, want 2", len(state.SetCookies))
	}
}

func TestSerializableRequest_Structure(t *testing.T) {
	// Test that SerializableRequest structure is correct
	req := SerializableRequest{
		Method: "POST",
		URL:    "https://example.com/api",
		Header: map[string][]string{
			"Content-Type":  {"application/json"},
			"Authorization": {"Bearer token"},
		},
	}

	if req.Method != "POST" {
		t.Errorf("Method = %q, want %q", req.Method, "POST")
	}
	if req.URL != "https://example.com/api" {
		t.Errorf("URL = %q, want %q", req.URL, "https://example.com/api")
	}
	if len(req.Header) != 2 {
		t.Errorf("Header length = %d, want 2", len(req.Header))
	}
}

func TestGroupInfo_Structure(t *testing.T) {
	// Test GroupInfo structure
	iconURL := "https://example.com/icon.png"
	group := GroupInfo{
		ID:      "group-123",
		Name:    "Administrators",
		IconURL: &iconURL,
	}

	if group.ID != "group-123" {
		t.Errorf("ID = %q, want %q", group.ID, "group-123")
	}
	if group.Name != "Administrators" {
		t.Errorf("Name = %q, want %q", group.Name, "Administrators")
	}
	if group.IconURL == nil {
		t.Error("IconURL should not be nil")
	} else if *group.IconURL != iconURL {
		t.Errorf("IconURL = %q, want %q", *group.IconURL, iconURL)
	}

	// Test with nil IconURL
	groupNoIcon := GroupInfo{
		ID:      "group-456",
		Name:    "Users",
		IconURL: nil,
	}
	if groupNoIcon.IconURL != nil {
		t.Error("IconURL should be nil")
	}
}
