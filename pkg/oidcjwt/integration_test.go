package oidcjwt_test

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apioauthn "github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/oidcjwt"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
)

type integrationResolver struct {
	user *gwtypes.User
}

func (s *integrationResolver) ResolveOrCreate(context.Context, *gwtypes.Identity, string) (*gwtypes.User, error) {
	return s.user, nil
}

func buildIntegrationStack(t *testing.T, gwUser *gwtypes.User) (http.Handler, *testutil.TestIssuer, func(), *rsa.PrivateKey) {
	t.Helper()

	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-int")
	cfg := oidcjwt.Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
		AdminRoles:           []string{"admin"},
	}
	v, err := oidcjwt.NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	jwtAuth := oidcjwt.NewAuthenticator(cfg, v, &integrationResolver{user: gwUser})
	wrapped := apioauthn.NewAuthenticator(union.NewFailOnError(jwtAuth, apioauthn.Anonymous{}))
	az := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, false)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}/entries", func(w http.ResponseWriter, r *http.Request) {
		info, err := wrapped.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if !az.Authorize(r, info) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})

	return mux, issuer, cleanup, priv
}

func runWithRoles(t *testing.T, roles []string) (int, map[string]any) {
	t.Helper()

	gwUser := &gwtypes.User{ID: 42, Username: "alice", Email: "alice@example.com"}
	mux, issuer, cleanup, priv := buildIntegrationStack(t, gwUser)
	defer cleanup()

	tok := testutil.MintTestJWT(t, priv, "kid-int", issuer.URL, "obot-default", "user-int",
		60*time.Second, map[string]any{"eligible": true, "roles": roles, "email": "alice@example.com"})

	req := httptest.NewRequest("GET", "/api/system-mcp-catalogs/default/entries", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestIntegration_AdminRoleReachesCatalog(t *testing.T) {
	code, body := runWithRoles(t, []string{"admin"})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "items")
}

func TestIntegration_NonAdminForbiddenAtCatalog(t *testing.T) {
	code, _ := runWithRoles(t, []string{"user"})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestIntegration_EmptyRolesForbiddenAtCatalog(t *testing.T) {
	code, _ := runWithRoles(t, []string{})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestIntegration_UnauthenticatedForbiddenAtCatalog(t *testing.T) {
	mux, _, cleanup, _ := buildIntegrationStack(t, nil)
	defer cleanup()
	req := httptest.NewRequest("GET", "/api/system-mcp-catalogs/default/entries", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

var _ authenticator.Request = (*oidcjwt.Authenticator)(nil)
