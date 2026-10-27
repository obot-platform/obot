package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type challengeTokenStorage struct {
	recordingTokenStorage
	conf *oauth2.Config
}

func (s *challengeTokenStorage) GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error) {
	return s.conf, nil, nil
}

func TestStartOAuthFromChallengeNeverProbesMCPAndUsesCapturedScopes(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{
			true:  "well-known fallback",
			false: "captured metadata",
		}[fallback], func(t *testing.T) {
			var operations atomic.Int32
			var base string
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/mcp":
					operations.Add(1)
					http.Error(w, "must not probe rejected operation", http.StatusInternalServerError)
				case "/metadata", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"resource":              base + "/mcp",
						"authorization_servers": []string{base},
						"scopes_supported":      []string{"wrong-default"},
					}))
				case "/.well-known/oauth-authorization-server":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"issuer":                   base,
						"response_types_supported": []string{"code"},
						"authorization_endpoint":   base + "/authorize",
						"token_endpoint":           base + "/token",
					}))
				default:
					http.NotFound(w, r)
				}
			}))
			defer provider.Close()
			base = provider.URL
			callback := &oauthAuthorizeCallbackHandler{}
			metadata := base + "/metadata"
			if fallback {
				metadata = ""
			}

			authURL, err := startOAuthFromChallenge(t.Context(), provider.Client(), ServerConfig{URL: base + "/mcp"}, ClientOption{
				CallbackHandler: callback,
				ClientLookup: &oauthTestClientCredLookup{
					clientID:     "existing-client",
					clientSecret: "secret",
				},
				TokenStorage:     &recordingTokenStorage{},
				OAuthClientName:  "Obot",
				OAuthRedirectURL: "https://obot.example/oauth/mcp/callback",
			}, metadata, "read write")
			require.NoError(t, err)

			parsed, err := url.Parse(authURL)
			require.NoError(t, err)
			require.Equal(t, "read write", parsed.Query().Get("scope"))
			require.Equal(t, "existing-client", parsed.Query().Get("client_id"))
			require.NotEmpty(t, parsed.Query().Get("code_challenge"))
			require.Zero(t, operations.Load())

			for _, test := range []struct {
				name     string
				authURL  string
				tokenURL string
				want     string
			}{
				{
					name:     "same provider retains grant",
					authURL:  base + "/authorize",
					tokenURL: base + "/token",
					want:     "write read",
				},
				{
					name:     "changed authorization endpoint",
					authURL:  "https://other.test/authorize",
					tokenURL: base + "/token",
					want:     "write",
				},
				{
					name:     "changed token endpoint",
					authURL:  base + "/authorize",
					tokenURL: "https://other.test/token",
					want:     "write",
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					authURL, err := startOAuthFromChallenge(t.Context(), provider.Client(), ServerConfig{URL: base + "/mcp"}, ClientOption{
						CallbackHandler: &oauthAuthorizeCallbackHandler{},
						ClientLookup:    &oauthTestClientCredLookup{clientID: "existing-client"},
						TokenStorage: &challengeTokenStorage{conf: &oauth2.Config{
							Endpoint: oauth2.Endpoint{
								AuthURL:  test.authURL,
								TokenURL: test.tokenURL,
							},
							Scopes: []string{"read", "write"},
						}},
						OAuthClientName:  "Obot",
						OAuthRedirectURL: "https://obot.example/oauth/mcp/callback",
					}, metadata, "write")
					require.NoError(t, err)

					parsed, err := url.Parse(authURL)
					require.NoError(t, err)
					require.Equal(t, test.want, parsed.Query().Get("scope"))
					require.Zero(t, operations.Load())
				})
			}
		})
	}
}

func TestBearerParametersDoNotMixProviders(t *testing.T) {
	header := `Basic realm="ignored", Bearer resource_metadata="https://first.test/metadata", scope="read", Bearer resource_metadata="https://second.test/metadata", scope="write"`

	require.Equal(t, "https://first.test/metadata", parseResourceMetadata(header))
	require.Equal(t, "read", parseScopeFromAuthenticateHeader(header))
}
