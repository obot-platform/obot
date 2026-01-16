package proxy

import (
	"testing"
	"time"
)

func TestGetUsername(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name         string
		providerName string
		state        serializableState
		want         string
	}{
		{
			name:         "github provider uses preferred username",
			providerName: "github-auth-provider",
			state: serializableState{
				PreferredUsername: "githubuser123",
				User:              "userid456",
				Email:             "user@example.com",
			},
			want: "githubuser123",
		},
		{
			name:         "github provider with empty preferred username",
			providerName: "github-auth-provider",
			state: serializableState{
				PreferredUsername: "",
				User:              "userid456",
				Email:             "user@example.com",
			},
			want: "",
		},
		{
			name:         "non-github provider uses User field",
			providerName: "google-auth-provider",
			state: serializableState{
				PreferredUsername: "someusername",
				User:              "userid123",
				Email:             "user@example.com",
			},
			want: "userid123",
		},
		{
			name:         "non-github provider falls back to Email when User is empty",
			providerName: "azure-auth-provider",
			state: serializableState{
				PreferredUsername: "someusername",
				User:              "",
				Email:             "user@example.com",
			},
			want: "user@example.com",
		},
		{
			name:         "non-github provider with both User and Email empty",
			providerName: "custom-provider",
			state: serializableState{
				PreferredUsername: "someusername",
				User:              "",
				Email:             "",
			},
			want: "",
		},
		{
			name:         "non-github provider prefers User over Email",
			providerName: "okta-provider",
			state: serializableState{
				PreferredUsername: "ignored",
				User:              "okta-user-id",
				Email:             "user@company.com",
			},
			want: "okta-user-id",
		},
		{
			name:         "empty provider name uses User field",
			providerName: "",
			state: serializableState{
				PreferredUsername: "username",
				User:              "user123",
				Email:             "user@test.com",
			},
			want: "user123",
		},
		{
			name:         "github-like provider name (not exact match)",
			providerName: "github-auth-provider-custom",
			state: serializableState{
				PreferredUsername: "githubuser",
				User:              "userid",
				Email:             "user@example.com",
			},
			want: "userid",
		},
		{
			name:         "case sensitive github provider check",
			providerName: "GitHub-Auth-Provider",
			state: serializableState{
				PreferredUsername: "githubuser",
				User:              "userid",
				Email:             "user@example.com",
			},
			want: "userid",
		},
		{
			name:         "github provider with all fields populated",
			providerName: "github-auth-provider",
			state: serializableState{
				ExpiresOn:         &now,
				AccessToken:       "token123",
				PreferredUsername: "octocat",
				User:              "12345",
				Email:             "octocat@github.com",
				SetCookies:        []string{"cookie1=value1"},
			},
			want: "octocat",
		},
		{
			name:         "google provider with numeric user ID",
			providerName: "google-oauth2",
			state: serializableState{
				User:  "1234567890",
				Email: "user@gmail.com",
			},
			want: "1234567890",
		},
		{
			name:         "provider with whitespace in User field",
			providerName: "auth0",
			state: serializableState{
				User:  " user-with-spaces ",
				Email: "test@example.com",
			},
			want: " user-with-spaces ",
		},
		{
			name:         "provider with special characters in email",
			providerName: "custom",
			state: serializableState{
				User:  "",
				Email: "user+test@example.co.uk",
			},
			want: "user+test@example.co.uk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUsername(tt.providerName, tt.state)
			if got != tt.want {
				t.Errorf("getUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetUsernameProviderPrecedence(t *testing.T) {
	// Verify the comment in the code about ordering - GitHub always uses PreferredUsername
	t.Run("github precedence", func(t *testing.T) {
		state := serializableState{
			PreferredUsername: "github-username",
			User:              "user-id",
			Email:             "email@example.com",
		}

		got := getUsername("github-auth-provider", state)
		if got != "github-username" {
			t.Errorf("GitHub provider should use PreferredUsername, got %q", got)
		}
	})

	// Verify non-GitHub providers use User, then Email
	t.Run("non-github precedence: User first", func(t *testing.T) {
		state := serializableState{
			PreferredUsername: "preferred",
			User:              "user-id",
			Email:             "email@example.com",
		}

		got := getUsername("google-oauth2", state)
		if got != "user-id" {
			t.Errorf("Non-GitHub provider should use User when available, got %q", got)
		}
	})

	t.Run("non-github precedence: Email fallback", func(t *testing.T) {
		state := serializableState{
			PreferredUsername: "preferred",
			User:              "",
			Email:             "email@example.com",
		}

		got := getUsername("google-oauth2", state)
		if got != "email@example.com" {
			t.Errorf("Non-GitHub provider should fall back to Email when User is empty, got %q", got)
		}
	})
}
