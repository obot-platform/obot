package oauth

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestIsRedirectURIAllowed(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		manifest    types.OAuthClientManifest
		redirectURI string
		want        bool
	}{
		{
			name: "exact match allowed",
			manifest: types.OAuthClientManifest{
				RedirectURIs: []string{"https://client.example/callback"},
			},
			redirectURI: "https://client.example/callback",
			want:        true,
		},
		{
			name: "web client requires exact match",
			manifest: types.OAuthClientManifest{
				ApplicationType: "web",
				RedirectURIs:    []string{"http://127.0.0.1/callback"},
			},
			redirectURI: "http://127.0.0.1:49152/callback",
		},
		{
			name: "native loopback request may add port",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1/callback"},
			},
			redirectURI: "http://127.0.0.1:49152/callback",
			want:        true,
		},
		{
			name: "native ipv6 loopback request may add port",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://[::1]/callback"},
			},
			redirectURI: "http://[::1]:49152/callback",
			want:        true,
		},
		{
			name: "native localhost request may add port",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://localhost/callback"},
			},
			redirectURI: "http://localhost:49152/callback",
			want:        true,
		},
		{
			name: "native exact match remains allowed",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1/callback"},
			},
			redirectURI: "http://127.0.0.1/callback",
			want:        true,
		},
		{
			name: "native registered URI with port does not relax",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1:3000/callback"},
			},
			redirectURI: "http://127.0.0.1:49152/callback",
		},
		{
			name: "native non-loopback request does not relax",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"https://client.example/callback"},
			},
			redirectURI: "https://client.example:49152/callback",
		},
		{
			name: "native path must still match",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1/callback"},
			},
			redirectURI: "http://127.0.0.1:49152/other",
		},
		{
			name: "native query must still match",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1/callback?client=desktop"},
			},
			redirectURI: "http://127.0.0.1:49152/callback?client=mobile",
		},
		{
			name: "native host must still match",
			manifest: types.OAuthClientManifest{
				ApplicationType: "native",
				RedirectURIs:    []string{"http://127.0.0.1/callback"},
			},
			redirectURI: "http://127.0.0.2:49152/callback",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isRedirectURIAllowed(tt.manifest, tt.redirectURI); got != tt.want {
				t.Fatalf("allowed = %v, want %v", got, tt.want)
			}
		})
	}
}
