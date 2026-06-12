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
	gate := integrationAuthzGate{
		authn: wrapped,
		az:    az,
	}
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}/entries", gate.serveJSON(map[string]any{"items": []any{}}))
	mux.HandleFunc("GET /api/system-mcp-servers/{id}", gate.serveJSON(map[string]any{"id": "system-server"}))
	mux.HandleFunc("GET /api/mcp-servers/{mcpserver_id}", gate.serveJSON(map[string]any{"id": "user-server"}))

	return mux, issuer, cleanup, priv
}

type integrationAuthzGate struct {
	authn *apioauthn.Authenticator
	az    *authz.Authorizer
}

func (g integrationAuthzGate) serveJSON(body map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := g.authn.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if !g.az.Authorize(r, info) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}
}

func runPathWithRoles(t *testing.T, path string, roles []string) (int, map[string]any) {
	t.Helper()

	gwUser := &gwtypes.User{ID: 42, Username: "alice", Email: "alice@example.com"}
	mux, issuer, cleanup, priv := buildIntegrationStack(t, gwUser)
	defer cleanup()

	tok := testutil.MintTestJWT(t, priv, "kid-int", issuer.URL, "obot-default", "user-int",
		60*time.Second, map[string]any{"eligible": true, "roles": roles, "email": "alice@example.com"})

	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestIntegration_AdminRoleReachesCatalogAndMCP(t *testing.T) {
	for _, tt := range integrationRoutes() {
		t.Run(tt.name, func(t *testing.T) {
			code, body := runPathWithRoles(t, tt.path, []string{"admin"})
			require.Equal(t, http.StatusOK, code)
			assert.Contains(t, body, tt.bodyKey)
		})
	}
}

func TestIntegration_NonAdminForbiddenAtCatalogAndMCP(t *testing.T) {
	for _, tt := range integrationRoutes() {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := runPathWithRoles(t, tt.path, []string{"user"})
			assert.Equal(t, http.StatusForbidden, code)
		})
	}
}

func TestIntegration_EmptyRolesForbiddenAtCatalogAndMCP(t *testing.T) {
	for _, tt := range integrationRoutes() {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := runPathWithRoles(t, tt.path, []string{})
			assert.Equal(t, http.StatusForbidden, code)
		})
	}
}

func TestIntegration_UnauthenticatedForbiddenAtCatalogAndMCP(t *testing.T) {
	mux, _, cleanup, _ := buildIntegrationStack(t, nil)
	defer cleanup()
	for _, tt := range integrationRoutes() {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func integrationRoutes() []struct {
	name    string
	path    string
	bodyKey string
} {
	return []struct {
		name    string
		path    string
		bodyKey string
	}{
		{name: "catalog entries", path: "/api/system-mcp-catalogs/default/entries", bodyKey: "items"},
		{name: "system mcp server", path: "/api/system-mcp-servers/test-server", bodyKey: "id"},
		{name: "user mcp server", path: "/api/mcp-servers/test-server", bodyKey: "id"},
	}
}

var _ authenticator.Request = (*oidcjwt.Authenticator)(nil)
