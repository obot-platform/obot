package auth

import (
	"testing"
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
