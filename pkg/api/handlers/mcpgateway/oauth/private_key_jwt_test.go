package oauth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidatePrivateKeyJWT(t *testing.T) {
	t.Parallel()

	const (
		clientID      = "https://client.example/oauth/client.json"
		tokenEndpoint = "https://obot.example/oauth/token"
	)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	jwks, err := json.Marshal(jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{
			Key:       &key.PublicKey,
			KeyID:     "test-key",
			Algorithm: "RS256",
			Use:       "sig",
		}},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	h := &handler{
		oauthConfig: handlers.OAuthAuthorizationServerConfig{
			TokenEndpoint: tokenEndpoint,
			TokenEndpointAuthSigningAlgValuesSupported: []string{"RS256"},
		},
	}
	client := v1.OAuthClient{
		Name: clientID,
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				RedirectURIs:            []string{"http://127.0.0.1/callback"},
				TokenEndpointAuthMethod: "private_key_jwt",
				JWKS:                    string(jwks),
			},
		},
	}

	assertion := signClientAssertion(t, key, clientID, tokenEndpoint)
	form := url.Values{
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	}

	if err := h.validatePrivateKeyJWT(t.Context(), form, client, clientID, ""); err != nil {
		t.Fatalf("validate private_key_jwt: %v", err)
	}

	form.Set("client_assertion", signClientAssertion(t, key, clientID, "https://other.example/oauth/token"))
	if err := h.validatePrivateKeyJWT(t.Context(), form, client, clientID, ""); err == nil {
		t.Fatal("expected invalid audience to fail")
	}
}

func TestClientIDFromClientAssertion(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	const clientID = "https://client.example/oauth/client.json"
	assertion := signClientAssertion(t, key, clientID, "https://obot.example/oauth/token")
	got, err := clientIDFromClientAssertion(url.Values{
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	})
	if err != nil {
		t.Fatalf("client id from assertion: %v", err)
	}
	if got != clientID {
		t.Fatalf("expected client id %q, got %q", clientID, got)
	}
}

func TestTokenExtractsClientIDFromClientAssertion(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	jwks, err := json.Marshal(jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{
			Key:       &key.PublicKey,
			KeyID:     "test-key",
			Algorithm: "RS256",
			Use:       "sig",
		}},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	const clientName = "test-client"
	clientID := system.DefaultNamespace + ":" + clientName
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.OAuthClient{
		Namespace: system.DefaultNamespace,
		Name:      clientName,
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				RedirectURIs:            []string{"http://127.0.0.1/callback"},
				TokenEndpointAuthMethod: "private_key_jwt",
				JWKS:                    string(jwks),
			},
		},
	}).Build()

	form := url.Values{
		"grant_type":            {"unsupported"},
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {signClientAssertion(t, key, clientID, "https://obot.example/oauth/token")},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err = (&handler{
		oauthConfig: handlers.OAuthAuthorizationServerConfig{
			TokenEndpoint:          "https://obot.example/oauth/token",
			GrantTypesSupported:    []string{"authorization_code"},
			ScopesSupported:        []string{"profile"},
			ResponseTypesSupported: []string{"code"},
			TokenEndpointAuthMethodsSupported: []string{
				"client_secret_basic",
				"client_secret_post",
				"private_key_jwt",
				"none",
			},
		},
	}).token(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
	})
	if err == nil {
		t.Fatal("expected unsupported grant type error")
	}
	if !strings.Contains(err.Error(), "grant_type") {
		t.Fatalf("expected request to reach grant type validation, got %v", err)
	}
}

func TestTokenPrivateKeyJWTAudienceMatchesRoute(t *testing.T) {
	const (
		baseURL      = "https://obot.example.com"
		clientID     = "https://client.example/oauth/client.json"
		mcpID        = system.SystemMCPServerPrefix + "test"
		code         = "authorization-code"
		refreshToken = "old-refresh-token"
	)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       &key.PublicKey,
		KeyID:     "test-key",
		Algorithm: "RS256",
		Use:       "sig",
	}}})
	require.NoError(t, err)
	metadata, err := json.Marshal(map[string]any{
		"client_id":                  clientID,
		"client_name":                "Test CIMD Client",
		"redirect_uris":              []string{"https://client.example/callback"},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_method": "private_key_jwt",
		"jwks":                       json.RawMessage(jwks),
	})
	require.NoError(t, err)

	for _, tt := range []struct {
		name      string
		grantType string
		scoped    bool
	}{
		{
			name:      "authorization code scoped",
			grantType: "authorization_code",
			scoped:    true,
		},
		{
			name:      "refresh token scoped",
			grantType: "refresh_token",
			scoped:    true,
		},
		{
			name:      "authorization code unscoped",
			grantType: "authorization_code",
		},
		{
			name:      "refresh token unscoped",
			grantType: "refresh_token",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var grant kclient.Object
			if tt.grantType == "authorization_code" {
				grant = &v1.OAuthAuthRequest{
					Namespace: system.DefaultNamespace,
					Name:      "oauth-request",
					Spec: v1.OAuthAuthRequestSpec{
						ClientID:       clientID,
						Resource:       baseURL + "/mcp-connect/" + mcpID,
						HashedAuthCode: fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
						UserID:         42,
						MCPID:          mcpID,
					},
				}
			} else {
				grant = &v1.OAuthToken{
					Namespace: system.DefaultNamespace,
					Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
					Spec: v1.OAuthTokenSpec{
						ClientID: clientID,
						Resource: baseURL + "/mcp-connect/" + mcpID,
						UserID:   42,
						MCPID:    mcpID,
					},
				}
			}
			storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, grant)
			h := newTestCIMDHandler(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, clientID, req.URL.String())
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(string(metadata))),
				}, nil
			})
			h.baseURL = baseURL
			h.oauthConfig.TokenEndpoint = baseURL + "/oauth/token"
			h.tokenService = tokenService

			endpoint := baseURL + "/oauth/token"
			path := "/oauth/token"
			otherEndpoint := endpoint + "/" + mcpID
			if tt.scoped {
				endpoint = otherEndpoint
				path += "/" + mcpID
				otherEndpoint = baseURL + "/oauth/token"
			}

			requestToken := func(audience string) (types.OAuthToken, error) {
				form := url.Values{
					"grant_type":            {tt.grantType},
					"client_assertion_type": {clientAssertionTypeJWTBearer},
					"client_assertion":      {signClientAssertion(t, key, clientID, audience)},
				}
				if tt.grantType == "authorization_code" {
					form.Set("code", code)
				} else {
					form.Set("refresh_token", refreshToken)
				}
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tt.scoped {
					req.SetPathValue("mcp_id", mcpID)
				}
				recorder := httptest.NewRecorder()
				err := h.token(api.Context{
					ResponseWriter: recorder,
					Request:        req,
					Storage:        storage,
					GatewayClient:  gatewayClient,
				})
				if err != nil {
					return types.OAuthToken{}, err
				}
				var response types.OAuthToken
				return response, json.NewDecoder(recorder.Body).Decode(&response)
			}

			for _, audience := range []string{otherEndpoint, baseURL + "/oauth/token/unrelated"} {
				_, err := requestToken(audience)
				assertInvalidClientErr(t, err)
				require.Contains(t, err.Error(), "invalid audience")
			}

			response, err := requestToken(endpoint)
			require.NoError(t, err)
			require.NotEmpty(t, response.AccessToken)
			require.NotEmpty(t, response.RefreshToken)
			if tt.grantType == "refresh_token" {
				require.NotEqual(t, refreshToken, response.RefreshToken)
			}
		})
	}
}

func TestTokenInvalidClientErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing credentials", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader("grant_type=authorization_code"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		err := (&handler{}).token(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
		})
		assertInvalidClientErr(t, err)
	})

	t.Run("unknown client", func(t *testing.T) {
		t.Parallel()

		form := url.Values{
			"grant_type": {"authorization_code"},
			"client_id":  {system.DefaultNamespace + ":missing-client"},
		}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		err := (&handler{
			oauthConfig: handlers.OAuthAuthorizationServerConfig{
				ScopesSupported:                   []string{"profile"},
				ResponseTypesSupported:            []string{"code"},
				TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "private_key_jwt", "none"},
			},
		}).token(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
		})
		assertInvalidClientErr(t, err)
	})

	t.Run("invalid client secret", func(t *testing.T) {
		t.Parallel()

		secretHash, err := bcrypt.GenerateFromPassword([]byte("correct-secret"), bcrypt.DefaultCost)
		if err != nil {
			t.Fatalf("hash secret: %v", err)
		}
		const clientName = "test-client"
		storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.OAuthClient{
			Namespace: system.DefaultNamespace,
			Name:      clientName,
			Spec: v1.OAuthClientSpec{
				ClientSecretHash: secretHash,
				Manifest: types.OAuthClientManifest{
					RedirectURIs:            []string{"http://127.0.0.1/callback"},
					TokenEndpointAuthMethod: "client_secret_post",
				},
			},
		}).Build()

		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {system.DefaultNamespace + ":" + clientName},
			"client_secret": {"wrong-secret"},
		}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		err = (&handler{
			oauthConfig: handlers.OAuthAuthorizationServerConfig{
				ScopesSupported:                   []string{"profile"},
				ResponseTypesSupported:            []string{"code"},
				TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "private_key_jwt", "none"},
			},
		}).token(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        storage,
		})
		assertInvalidClientErr(t, err)
	})
}

func assertInvalidClientErr(t *testing.T, err error) {
	t.Helper()

	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) {
		t.Fatalf("expected ErrHTTP, got %T: %v", err, err)
	}
	if errHTTP.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, errHTTP.Code)
	}
	if !strings.Contains(errHTTP.Message, `"error":"invalid_client"`) {
		t.Fatalf("expected invalid_client error, got %s", errHTTP.Message)
	}
	var oauthErr oauthError
	if err := json.Unmarshal([]byte(errHTTP.Message), &oauthErr); err != nil {
		t.Fatalf("expected valid OAuth error JSON, got %q: %v", errHTTP.Message, err)
	}
	if !strings.HasPrefix(oauthErr.Description, obotErrorPrefix) {
		t.Fatalf("expected Obot error marker, got %q", oauthErr.Description)
	}
}

func signClientAssertion(t *testing.T, key *rsa.PrivateKey, clientID, audience string) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Issuer:    clientID,
		Subject:   clientID,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ID:        "assertion-id",
	}
	tkn := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tkn.Header["kid"] = "test-key"

	assertion, err := tkn.SignedString(key)
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	return assertion
}
