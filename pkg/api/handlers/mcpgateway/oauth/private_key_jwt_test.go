package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
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

	if err := h.validatePrivateKeyJWT(context.Background(), form, client); err != nil {
		t.Fatalf("validate private_key_jwt: %v", err)
	}

	form.Set("client_assertion", signClientAssertion(t, key, "test-key", clientID, "https://other.example/oauth/token"))
	if err := h.validatePrivateKeyJWT(context.Background(), form, client); err == nil {
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

	const clientName = "test-client"
	clientID := system.DefaultNamespace + ":" + clientName
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      clientName,
		},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				TokenEndpointAuthMethod: "private_key_jwt",
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
			GrantTypesSupported: []string{"authorization_code"},
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
