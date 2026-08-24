package auth

import (
	"testing"
)

func TestGroupIDPrefixForAuthProvider(t *testing.T) {
	tests := []struct {
		name             string
		authProviderName string
		want             string
		wantError        bool
	}{
		{
			name:             "entra",
			authProviderName: "entra-auth-provider",
			want:             "entra/",
		},
		{
			name:             "provider name with hyphens",
			authProviderName: "custom-oidc-auth-provider",
			want:             "custom-oidc/",
		},
		{
			name:             "missing suffix",
			authProviderName: "entra",
			wantError:        true,
		},
		{
			name:             "empty prefix",
			authProviderName: "-auth-provider",
			wantError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GroupIDPrefixForAuthProvider(tt.authProviderName)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("prefix = %q, want %q", got, tt.want)
			}
		})
	}
}
