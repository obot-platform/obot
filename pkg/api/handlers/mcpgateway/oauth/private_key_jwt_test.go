package oauth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
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
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		ObjectMeta: metav1.ObjectMeta{Name: clientID},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				RedirectURIs:            []string{"http://127.0.0.1/callback"},
				TokenEndpointAuthMethod: "private_key_jwt",
				JWKS:                    string(jwks),
			},
		},
	}

	assertion := signClientAssertion(t, key, "test-key", clientID, tokenEndpoint)
	form := url.Values{
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	}

	if err := h.validatePrivateKeyJWT(t.Context(), form, client, clientID); err != nil {
		t.Fatalf("validate private_key_jwt: %v", err)
	}

	form.Set("client_assertion", signClientAssertion(t, key, "test-key", clientID, "https://other.example/oauth/token"))
	if err := h.validatePrivateKeyJWT(t.Context(), form, client, clientID); err == nil {
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
	assertion := signClientAssertion(t, key, "test-key", clientID, "https://obot.example/oauth/token")
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
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      clientName,
		},
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
		"client_assertion":      {signClientAssertion(t, key, "test-key", clientID, "https://obot.example/oauth/token")},
	}
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
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

func TestTokenInvalidClientErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing credentials", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader("grant_type=authorization_code"))
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
		req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
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
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.DefaultNamespace,
				Name:      clientName,
			},
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
		req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
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
}

func signClientAssertion(t *testing.T, key *rsa.PrivateKey, kid, clientID, audience string) string {
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
	tkn.Header["kid"] = kid

	assertion, err := tkn.SignedString(key)
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	return assertion
}
