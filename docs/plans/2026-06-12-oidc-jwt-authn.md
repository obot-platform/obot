# OIDC JWT Authenticator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic JWT authenticator to Obot that accepts JWTs issued by the configured `generic-oauth-auth-provider`. Each JWT carries a single subject shape (a user identity) plus two claims — `eligible` (boolean gate) and `roles` (array of Obot-vocabulary role names). The authenticator resolves or creates the Obot user, refuses if `eligible` is false, and maps `roles` to Obot's existing groups (`admin` → `[Admin, Owner, Authenticated]`; otherwise `[Authenticated]`). The first consumer is Studio's MCP integration — Studio plays the same role as Claude Desktop with an Obot API key, but presents a Studio-signed JWT.

**Architecture:** A new `pkg/oidcjwt/` package holds all integration code — config, a thin `go-oidc`-backed verifier wrapper, and an authenticator implementing the k8s `authenticator.Request` interface. JWT signature validation, OIDC discovery, JWKS caching, and key rotation are owned by `github.com/coreos/go-oidc/v3`. The authenticator is appended to the existing authenticator union in `pkg/services/config.go` with one additive block. The roles-to-groups mapping reuses Obot's existing `adminAndOwnerRules` (`pkg/api/authz/authz.go:26`) without any authz changes — a JWT whose `roles` claim contains a value in the configured admin-role list (default `["admin"]`) gets `[Admin, Owner, Authenticated]` groups; everything else gets `[Authenticated]`. User-subject JWTs resolve through the existing identity layer (`pkg/gateway/client/identity.go`), creating the Obot user record on first call if absent.

**Tech Stack:** Go (same toolchain as Obot today). New dependency: `github.com/coreos/go-oidc/v3`. Existing dep reused: `github.com/golang-jwt/jwt/v5` (for `ParseUnverified` in the issuer pre-check). Tests use `testify`. Integration test signs JWTs with a generated RSA keypair against an `httptest.Server` that serves an OIDC discovery doc.

**Design source of truth:** `docs/design/oidc-jwt-authn/README.md` (this repo).

---

## File Structure

| Path | Status | Responsibility |
|---|---|---|
| `pkg/oidcjwt/doc.go` | new | Package doc comment. |
| `pkg/oidcjwt/config.go` | new | Typed config struct, env-var binding. |
| `pkg/oidcjwt/config_test.go` | new | Tests for config parsing. |
| `pkg/oidcjwt/testutil/testutil.go` | new | Shared test helpers: `NewTestIssuer`, `MintTestJWT`, `MustRSAKey`. |
| `pkg/oidcjwt/verifier.go` | new | Thin wrapper around `go-oidc`'s `*oidc.Provider` + `*oidc.IDTokenVerifier`. Handles the "is this JWT ours?" pre-check (parses `iss` without verification, compares to configured issuer). |
| `pkg/oidcjwt/verifier_test.go` | new | Tests for the verifier wrapper. |
| `pkg/oidcjwt/authenticator.go` | new | Implements `authenticator.Request`. Composes config + verifier + identity resolution + role-to-group mapping. |
| `pkg/oidcjwt/authenticator_test.go` | new | Unit tests (admin role → admin/owner groups, no admin role → authenticated only, missing eligibility → 401, different issuer → fall through). |
| `pkg/oidcjwt/identity_adapter.go` | new | Maps a validated JWT to an Obot user record via `pkg/gateway/client.EnsureIdentity`. |
| `pkg/oidcjwt/integration_test.go` | new | End-to-end test: real `authn.Authenticator`, JWT with `roles: ["admin"]` → handler returns 200; JWT with empty roles → handler returns 403. |
| `pkg/services/config.go` | modify (one block) | Load `oidcjwt.Config`, construct verifier, append `oidcjwt.NewAuthenticator(...)` to the authenticators union when enabled. |
| `chart/values.yaml` | modify | Add 4 new `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*` keys. |
| `go.mod` / `go.sum` | modify | Add `github.com/coreos/go-oidc/v3`. |
| `docs/studio/CHANGES.md` | new | Upstream-touchpoint manifest. |
| `scripts/check-upstream-touchpoints.sh` | new | CI check that flags unexpected upstream touches. |

The upstream-touch allow-list: `pkg/services/config.go`, `chart/values.yaml`, `go.mod`, `go.sum`, plus the doc/script files. Everything else lives under `pkg/oidcjwt/`.

---

## Task 1: Add `coreos/go-oidc` dependency and scaffold the package

**Files:** `go.mod`, `go.sum`, `pkg/oidcjwt/doc.go`

- [ ] **Step 1:** `go get github.com/coreos/go-oidc/v3@latest`
Expected: `go.mod` updated; no errors.

- [ ] **Step 2:** Verify the dep landed.
Run: `grep coreos/go-oidc go.mod`
Expected: `github.com/coreos/go-oidc/v3 vX.Y.Z`.

- [ ] **Step 3:** Create `pkg/oidcjwt/doc.go`:

```go
// Package oidcjwt implements a generic JWT authenticator that validates
// JWTs issued by the configured generic-oauth-auth-provider. See
// docs/design/oidc-jwt-authn/README.md for the contract.
package oidcjwt
```

- [ ] **Step 4:** `go build ./pkg/oidcjwt/...`
Expected: clean.

- [ ] **Step 5:** Commit.

```bash
git add go.mod go.sum pkg/oidcjwt/doc.go
git commit -m "feat(oidcjwt): scaffold package with coreos/go-oidc dep"
```

---

## Task 2: Config with env-var binding

**Files:** `pkg/oidcjwt/config.go`, `pkg/oidcjwt/config_test.go`

- [ ] **Step 1: Write the failing test**

Path: `pkg/oidcjwt/config_test.go`

```go
package oidcjwt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromEnv_AllFieldsPresent(t *testing.T) {
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":                 "https://studio.example.com/api/auth",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE":               "obot-default",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME": "eligible",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME":       "roles",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES":            "admin,owner",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.Equal(t, "https://studio.example.com/api/auth", cfg.IssuerURL)
	assert.Equal(t, "obot-default", cfg.Audience)
	assert.Equal(t, "eligible", cfg.EligibilityClaimName)
	assert.Equal(t, "roles", cfg.RolesClaimName)
	assert.Equal(t, []string{"admin", "owner"}, cfg.AdminRoles)
	assert.True(t, cfg.Enabled())
}

func TestLoadConfigFromEnv_DefaultsAndDisabled(t *testing.T) {
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER": "https://studio.example.com/api/auth",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.False(t, cfg.Enabled())                        // empty audience -> disabled
	assert.Equal(t, "eligible", cfg.EligibilityClaimName) // default
	assert.Equal(t, "roles", cfg.RolesClaimName)          // default
	assert.Equal(t, []string{"admin"}, cfg.AdminRoles)    // default
}

func envGetter(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}
```

- [ ] **Step 2:** `go test ./pkg/oidcjwt/... -run TestLoadConfigFromEnv -v`
Expected: FAIL (`undefined: LoadConfigFromEnv`).

- [ ] **Step 3: Implement**

Path: `pkg/oidcjwt/config.go`

```go
package oidcjwt

import "strings"

// Config holds the runtime configuration for the JWT authenticator.
// All fields source from the existing OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*
// env-var prefix.
type Config struct {
	// IssuerURL is the raw value as configured (trailing slash preserved if
	// present). It is what we store on the Identity (ProviderIssuer) and use
	// to compose ProviderUserID — see Identity convergence note in
	// docs/design/oidc-jwt-authn/README.md.
	IssuerURL string

	// NormalizedIssuerURL is IssuerURL with whitespace and trailing slashes
	// trimmed. Used ONLY for the pre-check comparison against a JWT's iss
	// claim so https://issuer/ and https://issuer match. Mirrors the
	// generic-oauth-auth-provider's normalizedIssuer() helper in
	// providers/generic-oauth-auth-provider/main.go.
	NormalizedIssuerURL string

	Audience             string
	EligibilityClaimName string
	RolesClaimName       string
	AdminRoles           []string
}

const (
	defaultEligibilityClaimName = "eligible"
	defaultRolesClaimName       = "roles"
)

var defaultAdminRoles = []string{"admin"}

// NormalizeIssuer trims whitespace and trailing slashes. Matches the
// existing provider behavior so identity convergence is exact.
func NormalizeIssuer(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// Enabled reports whether the authenticator is functional.
func (c Config) Enabled() bool {
	return c.IssuerURL != "" && c.Audience != ""
}

// LoadConfigFromEnv reads OBOT_GENERIC_OAUTH_AUTH_PROVIDER_* env vars
// via the supplied getter. Missing optional values fall back to defaults.
func LoadConfigFromEnv(getenv func(string) string) (Config, error) {
	issuer := getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER")
	cfg := Config{
		IssuerURL:            issuer,
		NormalizedIssuerURL:  NormalizeIssuer(issuer),
		Audience:             getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE"),
		EligibilityClaimName: getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME"),
		RolesClaimName:       getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME"),
	}
	if cfg.EligibilityClaimName == "" {
		cfg.EligibilityClaimName = defaultEligibilityClaimName
	}
	if cfg.RolesClaimName == "" {
		cfg.RolesClaimName = defaultRolesClaimName
	}
	adminRolesStr := getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES")
	if adminRolesStr == "" {
		cfg.AdminRoles = defaultAdminRoles
	} else {
		for _, r := range strings.Split(adminRolesStr, ",") {
			if trimmed := strings.TrimSpace(r); trimmed != "" {
				cfg.AdminRoles = append(cfg.AdminRoles, trimmed)
			}
		}
	}
	return cfg, nil
}
```

Add a test for normalization:

```go
func TestNormalizeIssuer(t *testing.T) {
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("https://issuer.example.com/"))
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("  https://issuer.example.com/  "))
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("https://issuer.example.com//"))
}

func TestLoadConfigFromEnv_NormalizesIssuer(t *testing.T) {
	cfg, err := LoadConfigFromEnv(envGetter(map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":   "https://issuer.example.com/",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE": "obot-default",
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://issuer.example.com/", cfg.IssuerURL)            // raw preserved
	assert.Equal(t, "https://issuer.example.com", cfg.NormalizedIssuerURL)  // trimmed
}
```

- [ ] **Step 4:** `go test ./pkg/oidcjwt/... -run TestLoadConfigFromEnv -v`
Expected: 2 PASS.

- [ ] **Step 5:** Commit.

```bash
git add pkg/oidcjwt/config.go pkg/oidcjwt/config_test.go
git commit -m "feat(oidcjwt): config with env-var binding (generic claim names + admin-roles list)"
```

---

## Task 3: Test helpers (`pkg/oidcjwt/testutil/`)

**Files:** `pkg/oidcjwt/testutil/testutil.go`

- [ ] **Step 1: Write the helpers**

Path: `pkg/oidcjwt/testutil/testutil.go`

```go
// Package testutil provides shared test helpers for pkg/oidcjwt: an
// in-process OIDC issuer (discovery doc + JWKS) and a JWT minter.
package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type TestIssuer struct {
	*httptest.Server
	mu   sync.Mutex
	keys map[string]*rsa.PrivateKey
}

func NewTestIssuer(t *testing.T, priv *rsa.PrivateKey, kid string) (*TestIssuer, func()) {
	ti := &TestIssuer{keys: map[string]*rsa.PrivateKey{kid: priv}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                base,
			"jwks_uri":                              base + "/.well-known/jwks.json",
			"authorization_endpoint":                base + "/auth",
			"token_endpoint":                        base + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		ti.mu.Lock()
		defer ti.mu.Unlock()
		jwks := struct {
			Keys []map[string]string `json:"keys"`
		}{}
		for kid, p := range ti.keys {
			pub := p.PublicKey
			jwks.Keys = append(jwks.Keys, map[string]string{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_ = json.NewEncoder(w).Encode(jwks)
	})
	srv := httptest.NewServer(mux)
	ti.Server = srv
	return ti, srv.Close
}

func (ti *TestIssuer) AddKey(t *testing.T, priv *rsa.PrivateKey, kid string) {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	ti.keys[kid] = priv
}

func MintTestJWT(t *testing.T, priv *rsa.PrivateKey, kid, iss, aud, sub string, ttl time.Duration, extra map[string]any) string {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": iss,
		"aud": aud,
		"sub": sub,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

func MustRSAKey(t *testing.T) *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}
```

- [ ] **Step 2:** `go build ./pkg/oidcjwt/testutil/...`
Expected: clean.

- [ ] **Step 3:** Commit.

```bash
git add pkg/oidcjwt/testutil/testutil.go
git commit -m "feat(oidcjwt): test helpers (in-process OIDC issuer, JWT minter)"
```

---

## Task 4: Verifier wrapper

**Files:** `pkg/oidcjwt/verifier.go`, `pkg/oidcjwt/verifier_test.go`

Wraps `go-oidc`'s `*oidc.Provider` + `*oidc.IDTokenVerifier`. Adds the "is this JWT ours?" pre-check.

- [ ] **Step 1: Write the failing test**

Path: `pkg/oidcjwt/verifier_test.go`

```go
package oidcjwt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifier_AcceptsValidToken(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
		"email":    "alice@example.com",
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.True(t, claims.Eligible)
	assert.Equal(t, []string{"admin"}, claims.Roles)
	assert.Equal(t, "alice@example.com", claims.Email)
}

func TestVerifier_ReturnsNotMineForDifferentIssuer(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", "https://other-issuer.example.com", "obot-default", "user-1", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	assert.True(t, errors.Is(err, ErrNotMyToken))
}

func TestVerifier_RejectsWrongAudience(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "wrong-aud", "user-1", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotMyToken))
}
```

- [ ] **Step 2:** `go test ./pkg/oidcjwt/... -run TestVerifier -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Path: `pkg/oidcjwt/verifier.go`

```go
package oidcjwt

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

// ErrNotMyToken signals that the JWT does not belong to this
// authenticator (different issuer, or not parseable). Callers fall
// through to the next authenticator in the union.
var ErrNotMyToken = errors.New("oidcjwt: token not for this authenticator")

// Claims is the validated set of claims this authenticator cares about.
// Mirrors the shape the existing generic-oauth-auth-provider browser
// flow extracts (see providers/generic-oauth-auth-provider/pkg/profile/
// profile.go and main.go) so identity convergence with browser-login is
// exact.
type Claims struct {
	// Raw issuer from the JWT (not normalized). Stored on the Identity
	// as ProviderIssuer to match what the browser flow writes.
	Issuer string

	Subject  string
	Audience string

	Eligible bool
	Roles    []string

	// Provider profile claims (same shape as profile.UserInfo in
	// providers/generic-oauth-auth-provider).
	Email             string
	EmailVerified     *bool
	PreferredUsername string
	Name              string
	Picture           string
}

type Verifier struct {
	cfg      Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	// Use the raw IssuerURL for discovery — go-oidc will validate the
	// discovery doc's `issuer` field matches.
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidcjwt: oidc discovery: %w", err)
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID:             cfg.Audience,
		SupportedSigningAlgs: []string{"RS256"},
	})
	return &Verifier{cfg: cfg, verifier: verifier, provider: provider}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		return Claims{}, ErrNotMyToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, ErrNotMyToken
	}
	iss, _ := mc["iss"].(string)
	// Compare normalized form to match the issuer-normalization the
	// browser-flow provider does (TrimRight "/"). Tokens minted with or
	// without a trailing slash on iss are accepted equivalently.
	if NormalizeIssuer(iss) != v.cfg.NormalizedIssuerURL {
		return Claims{}, ErrNotMyToken
	}

	idToken, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, fmt.Errorf("oidcjwt: verify: %w", err)
	}

	var custom struct {
		Email             string `json:"email"`
		EmailVerified     *bool  `json:"email_verified,omitempty"`
		PreferredUsername string `json:"preferred_username,omitempty"`
		Name              string `json:"name,omitempty"`
		Picture           string `json:"picture,omitempty"`
	}
	_ = idToken.Claims(&custom)

	aud := ""
	if a, ok := mc["aud"].(string); ok {
		aud = a
	}

	return Claims{
		Issuer:            idToken.Issuer, // raw issuer from JWT — store on Identity
		Subject:           idToken.Subject,
		Audience:          aud,
		Eligible:          readEligibility(mc, v.cfg.EligibilityClaimName),
		Roles:             readRoles(mc, v.cfg.RolesClaimName),
		Email:             custom.Email,
		EmailVerified:     custom.EmailVerified,
		PreferredUsername: custom.PreferredUsername,
		Name:              custom.Name,
		Picture:           custom.Picture,
	}, nil
}

func readEligibility(mc jwt.MapClaims, name string) bool {
	if name == "" {
		return false
	}
	switch v := mc[name].(type) {
	case bool:
		return v
	case []any:
		return len(v) > 0
	}
	return false
}

func readRoles(mc jwt.MapClaims, name string) []string {
	if name == "" {
		return nil
	}
	raw, ok := mc[name].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
```

Add a test asserting the issuer-normalization equivalence and the full claim extraction:

```go
func TestVerifier_AcceptsTokenWithTrailingSlashIssuerWhenConfigHasNoSlash(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	// Config without trailing slash; JWT with trailing slash.
	cfg := Config{
		IssuerURL:            issuer.URL,
		NormalizedIssuerURL:  NormalizeIssuer(issuer.URL),
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL+"/", "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
	})
	_, err = v.Verify(context.Background(), tok)
	// Whether this passes depends on what go-oidc's discovery returns
	// as the canonical `issuer`. The pre-check at least won't reject it
	// on the trailing slash alone.
	// (Real go-oidc may still reject if the JWT iss differs from the
	// discovery doc's issuer. Test against your provider's behavior.)
	assert.NoError(t, err)
}

func TestVerifier_ExtractsFullProviderProfile(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		NormalizedIssuerURL:  NormalizeIssuer(issuer.URL),
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	emailVerified := true
	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible":           true,
		"roles":              []string{"admin"},
		"email":              "alice@example.com",
		"email_verified":     emailVerified,
		"preferred_username": "alice",
		"name":               "Alice Example",
		"picture":            "https://example.com/alice.png",
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", claims.Email)
	require.NotNil(t, claims.EmailVerified)
	assert.True(t, *claims.EmailVerified)
	assert.Equal(t, "alice", claims.PreferredUsername)
	assert.Equal(t, "Alice Example", claims.Name)
	assert.Equal(t, "https://example.com/alice.png", claims.Picture)
}
```

- [ ] **Step 4:** `go test ./pkg/oidcjwt/... -run TestVerifier -v`
Expected: 3 PASS.

- [ ] **Step 5:** Commit.

```bash
git add pkg/oidcjwt/verifier.go pkg/oidcjwt/verifier_test.go
git commit -m "feat(oidcjwt): verifier wrapper around coreos/go-oidc"
```

---

## Task 5: Authenticator with role-to-group mapping

**Files:** `pkg/oidcjwt/authenticator.go`, `pkg/oidcjwt/authenticator_test.go`, `pkg/oidcjwt/identity_adapter.go`

- [ ] **Step 1: Survey the identity layer**

Run: `grep -n "^func .* Client.*EnsureIdentity\|^type Identity " pkg/gateway/client/identity.go apiclient/types/identity.go`
Expected: current `EnsureIdentity` signature and `types.Identity` field shapes.

- [ ] **Step 2: Write the failing test**

Path: `pkg/oidcjwt/authenticator_test.go`

```go
package oidcjwt

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticator_AdminRoleGrantsAdminOwner(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(&gwtypes.User{ID: 1, Username: "alice", Email: "alice@example.com"}))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
	})
	req, _ := http.NewRequest("GET", "/api/system-mcp-catalogs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.Contains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAuthenticated)

	// Final user.Info shape matches what APIKeyAuthenticator produces:
	assert.Equal(t, "1", resp.User.GetUID())
	assert.Equal(t, "alice", resp.User.GetName())

	// Extras carry the auth_provider_* fields handlers expect.
	extra := resp.User.GetExtra()
	assert.Equal(t, []string{"alice@example.com"}, extra["email"])
	assert.Equal(t, []string{"generic-oauth-auth-provider"}, extra["auth_provider_name"])
	require.NotEmpty(t, extra["auth_provider_user_id"])
	assert.Contains(t, extra["auth_provider_user_id"][0], "iss:")
	assert.Contains(t, extra["auth_provider_user_id"][0], "\x00sub:user-1")
}

func TestBuildIdentity_UsernameFallback(t *testing.T) {
	cfg := Config{IssuerURL: "https://issuer.example.com"}

	cases := []struct {
		name       string
		claims     Claims
		wantUser   string
	}{
		{"preferred_username wins", Claims{Subject: "s", Email: "e@x", Name: "N", PreferredUsername: "p"}, "p"},
		{"name when no preferred", Claims{Subject: "s", Email: "e@x", Name: "N"}, "N"},
		{"email when no name", Claims{Subject: "s", Email: "e@x"}, "e@x"},
		{"sub when nothing else", Claims{Subject: "s"}, "s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.claims.Issuer = "https://issuer.example.com"
			id := buildIdentity(cfg, tc.claims)
			assert.Equal(t, tc.wantUser, id.ProviderUsername)
		})
	}
}

func TestBuildIdentity_ProviderUserIDFormat(t *testing.T) {
	cfg := Config{IssuerURL: "https://issuer.example.com/"}
	claims := Claims{Issuer: "https://issuer.example.com/", Subject: "user-1"}
	id := buildIdentity(cfg, claims)
	assert.Equal(t, "iss:https://issuer.example.com/\x00sub:user-1", id.ProviderUserID)
	assert.Equal(t, "https://issuer.example.com/", id.ProviderIssuer)
}

func TestAuthenticator_NonAdminRoleGetsAuthenticatedOnly(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(&gwtypes.User{ID: 2, Username: "bob"}))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-2", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"user"},
	})
	req, _ := http.NewRequest("GET", "/api/mcp-servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.NotContains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAuthenticated)
}

func TestAuthenticator_FailsWhenIneligible(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(nil))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-3", 60*time.Second, map[string]any{
		"eligible": false,
	})
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err = auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

func TestAuthenticator_NoBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, nil, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_DifferentIssuerFallsThrough(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(nil))

	tok := testutil.MintTestJWT(t, priv, "kid-X", "https://other-issuer.example.com", "obot-default", "user-4", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

type stubResolverImpl struct {
	out *gwtypes.User
	err error
}

func (s *stubResolverImpl) ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.out == nil {
		return nil, errors.New("stub: no user")
	}
	// Optionally assert the Identity built by the authenticator looks
	// right: ProviderUserID format, ProviderIssuer = raw issuer, etc.
	return s.out, nil
}

func stubResolver(out *gwtypes.User) IdentityResolver {
	return &stubResolverImpl{out: out}
}
```

- [ ] **Step 3:** `go test ./pkg/oidcjwt/... -run TestAuthenticator -v`
Expected: FAIL.

- [ ] **Step 4: Implement the authenticator**

Path: `pkg/oidcjwt/authenticator.go`

```go
package oidcjwt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// IdentityResolver maps a validated JWT to an Obot gateway user. The
// implementation owns the get-or-create call against the gateway
// identity store.
type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error)
}

// Authenticator implements k8s.io/apiserver/pkg/authentication/authenticator.Request.
//
// MUST be inserted into the union AFTER client.NewUserDecorator so the
// decorator does not rewrap the response. This authenticator produces
// the final user.Info itself — UID, Name, Groups, Extra — using the
// same shape gateway.server.APIKeyAuthenticator uses (see
// pkg/gateway/server/apikey_auth.go).
type Authenticator struct {
	cfg      Config
	verifier *Verifier
	identity IdentityResolver
}

func NewAuthenticator(cfg Config, verifier *Verifier, identity IdentityResolver) *Authenticator {
	return &Authenticator{cfg: cfg, verifier: verifier, identity: identity}
}

// AuthenticateRequest implements authenticator.Request.
//
//   - (resp, true, nil) on a fully-validated JWT with eligibility=true.
//   - (nil, false, nil) when the token does not belong to us.
//   - (nil, false, err) on a real auth failure (ours but invalid, or
//     eligibility false).
func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !a.cfg.Enabled() || a.verifier == nil {
		return nil, false, nil
	}
	raw := bearerToken(req)
	if raw == "" {
		return nil, false, nil
	}

	claims, err := a.verifier.Verify(req.Context(), raw)
	if errors.Is(err, ErrNotMyToken) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: %w", err)
	}

	if !claims.Eligible {
		return nil, false, errors.New("oidcjwt: eligibility claim missing or false")
	}
	if a.identity == nil {
		return nil, false, errors.New("oidcjwt: identity resolver not configured")
	}

	id := buildIdentity(a.cfg, claims)
	timezone := req.Header.Get("X-Obot-User-Timezone")

	gwUser, err := a.identity.ResolveOrCreate(req.Context(), id, timezone)
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: identity resolve: %w", err)
	}

	groups := deriveGroups(claims.Roles, a.cfg.AdminRoles)

	// Extras match the shape the existing UserDecorator sets so any
	// downstream handler that inspects `extra` (e.g. for email or
	// auth_provider_*) sees the same keys it would for a browser user.
	// We omit auth_provider_groups since this authenticator is not yet
	// integrated with the auth-provider group lookup; callers that need
	// it can call gatewayClient.ListGroupIDsForUser themselves.
	extra := map[string][]string{
		"email":                        {gwUser.Email},
		"auth_provider_name":           {genericOAuthAuthProviderName},
		"auth_provider_namespace":      {genericOAuthAuthProviderNamespace},
		"auth_provider_issuer":         {claims.Issuer}, // raw, matching browser flow
		"auth_provider_user_id":        {id.ProviderUserID},
	}
	if claims.EmailVerified != nil {
		if *claims.EmailVerified {
			extra["auth_provider_email_verified"] = []string{"true"}
		} else {
			extra["auth_provider_email_verified"] = []string{"false"}
		}
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   gwUser.Username,
			UID:    fmt.Sprintf("%d", gwUser.ID),
			Groups: groups,
			Extra:  extra,
		},
	}, true, nil
}

// buildIdentity composes the *gateway/types.Identity from the JWT
// claims using the exact same shape the browser-flow provider produces.
// Identity convergence depends on this — see
// docs/design/oidc-jwt-authn/README.md §_Identity mapping_.
//
// In particular:
//   - ProviderUserID = "iss:<raw issuer>\x00sub:<sub>"
//   - ProviderIssuer = raw issuer (with whatever trailing slash the
//     issuer carries — matches existing test fixtures)
//   - ProviderUsername follows the provider's fallback rule:
//     preferred_username → name → email → sub
func buildIdentity(cfg Config, claims Claims) *gwtypes.Identity {
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Subject
	}
	providerUserID := "iss:" + claims.Issuer + "\x00sub:" + claims.Subject
	return &gwtypes.Identity{
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: genericOAuthAuthProviderNamespace,
		ProviderUsername:      username,
		ProviderUserID:        providerUserID,
		ProviderIssuer:        claims.Issuer,
		ProviderEmailVerified: claims.EmailVerified,
		Email:                 claims.Email,
	}
}

func deriveGroups(jwtRoles, adminRoles []string) []string {
	adminSet := make(map[string]bool, len(adminRoles))
	for _, r := range adminRoles {
		adminSet[r] = true
	}
	for _, r := range jwtRoles {
		if adminSet[r] {
			return []string{types.GroupAdmin, types.GroupOwner, types.GroupAuthenticated}
		}
	}
	return []string{types.GroupAuthenticated}
}

func bearerToken(req *http.Request) string {
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// Constants pulled in to avoid taking a dependency on
// pkg/controller/handlers/provider just for two strings. Cross-check
// against pkg/controller/handlers/provider/env_auth_provider.go and
// pkg/system at implementation time.
const (
	genericOAuthAuthProviderName      = "generic-oauth-auth-provider"
	genericOAuthAuthProviderNamespace = "default" // = system.DefaultNamespace
)
```

> **Note on `system.DefaultNamespace`:** the literal "default" is used in tests (`pkg/gateway/client/identity_generic_oauth_test.go:21` uses `system.DefaultNamespace`). Reference `pkg/system` directly in the implementation rather than hard-coding the literal. The constant is named here as a placeholder for the actual import.

- [ ] **Step 5: Implement the identity adapter**

Path: `pkg/oidcjwt/identity_adapter.go`

```go
package oidcjwt

import (
	"context"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// NewGatewayIdentityResolver returns an IdentityResolver backed by the
// gateway client's EnsureIdentity path — the same path the browser
// OAuth provider uses at user-login time. The Identity passed in must
// already carry AuthProviderName, AuthProviderNamespace, ProviderUserID
// (in "iss:...\x00sub:..." shape), ProviderIssuer, ProviderUsername,
// Email, and ProviderEmailVerified populated. The authenticator
// (Authenticator.AuthenticateRequest, via buildIdentity) is responsible
// for that population; this adapter only forwards.
func NewGatewayIdentityResolver(c *gclient.Client) IdentityResolver {
	return &gatewayResolver{c: c}
}

type gatewayResolver struct{ c *gclient.Client }

func (g *gatewayResolver) ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error) {
	// EnsureIdentity(ctx, id, timezone) — confirmed signature in
	// pkg/gateway/client/identity.go:61 at the time of writing. It
	// returns the *gwtypes.User associated with the identity, creating
	// the row on first sight (same shape as the browser-flow first
	// login).
	return g.c.EnsureIdentity(ctx, id, timezone)
}
```

The adapter is a one-line passthrough. All the Identity construction logic lives in `Authenticator.buildIdentity` so it's testable without a real gateway client.

- [ ] **Step 6:** `go test ./pkg/oidcjwt/... -v`
Expected: all PASS.

- [ ] **Step 7:** Commit.

```bash
git add pkg/oidcjwt/authenticator.go pkg/oidcjwt/authenticator_test.go pkg/oidcjwt/identity_adapter.go
git commit -m "feat(oidcjwt): authenticator with role-to-group mapping"
```

---

## Task 6: Wire into the authenticator union

**Files:** `pkg/services/config.go` (one block)

- [ ] **Step 1:** Locate the union-build region.
Run: `sed -n '825,850p' pkg/services/config.go`

- [ ] **Step 2:** Insert after `authenticators = union.New(authenticators, persistentTokenServer)` (around line 840):

```go
// OIDC JWT authenticator — accepts JWTs from the configured
// generic-oauth-auth-provider. See pkg/oidcjwt and
// docs/design/oidc-jwt-authn/README.md.
oidcJWTCfg, err := oidcjwt.LoadConfigFromEnv(os.Getenv)
if err != nil {
    return nil, fmt.Errorf("oidcjwt config: %w", err)
}
if oidcJWTCfg.Enabled() {
    oidcVerifier, err := oidcjwt.NewVerifier(ctx, oidcJWTCfg)
    if err != nil {
        return nil, fmt.Errorf("oidcjwt verifier: %w", err)
    }
    authenticators = union.New(authenticators,
        oidcjwt.NewAuthenticator(oidcJWTCfg, oidcVerifier, oidcjwt.NewGatewayIdentityResolver(gatewayClient)))
}
```

Add import `"github.com/obot-platform/obot/pkg/oidcjwt"`.

- [ ] **Step 3:** `go build ./...`
Expected: clean.

- [ ] **Step 4:** `go test ./pkg/...`
Expected: all pass.

- [ ] **Step 5:** Commit.

```bash
git add pkg/services/config.go
git commit -m "feat(oidcjwt): wire into authenticator union when configured"
```

---

## Task 7: End-to-end integration test

**Files:** `pkg/oidcjwt/integration_test.go`

- [ ] **Step 1: Write the test**

Path: `pkg/oidcjwt/integration_test.go`

The point of this test is to assert that a JWT presented at `GET /api/system-mcp-catalogs/{catalog_id}/entries` actually flows through Obot's real authorization rules — `adminAndOwnerRules` in `pkg/api/authz/authz.go:26` — not just through a fake handler that checks groups. That way a regression in either the authenticator wiring or the authz rule set is caught.

The closest existing testing pattern is `pkg/api/handlers/systemmcpcatalogs_test.go` — read it before writing this test to see how it wires up a request context against the real authorizer. The skeleton below uses `authz.NewAuthorizer` with stub clients and runs the request through both `authn.NewAuthenticator(jwtAuth)` and `authz.Authorize` so the assertion is "the real authz layer accepts/rejects."

```go
package oidcjwt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/oidcjwt"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

// stubResolver returns a fixed *gwtypes.User without going through a
// real gateway. The authenticator's buildIdentity is exercised; only
// the EnsureIdentity round-trip is stubbed.
type stubResolver struct{ user *gwtypes.User }

func (s *stubResolver) ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error) {
	return s.user, nil
}

// buildIntegrationStack wires:
//
//   - testutil.NewTestIssuer as the OIDC issuer
//   - oidcjwt.NewAuthenticator wrapped by authn.NewAuthenticator
//     (the same wrapper pkg/services/config.go uses)
//   - authz.NewAuthorizer with the stub kclient/gatewayClient that
//     existing handler tests use (see pkg/api/handlers/
//     systemmcpcatalogs_test.go for the pattern)
//
// Then registers a real handler (or mounts the real router subset) at
// /api/system-mcp-catalogs/{catalog_id}/entries that, before doing
// any work, asks the authorizer to check the request.
func buildIntegrationStack(t *testing.T, gwUser *gwtypes.User) (http.Handler, *testutil.TestIssuer, func(), oidcjwt.Config, /* test issuer priv */ any) {
	t.Helper()
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-int")

	cfg := oidcjwt.Config{
		IssuerURL:            issuer.URL,
		NormalizedIssuerURL:  oidcjwt.NormalizeIssuer(issuer.URL),
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
		AdminRoles:           []string{"admin"},
	}
	v, err := oidcjwt.NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	jwtAuth := oidcjwt.NewAuthenticator(cfg, v, &stubResolver{user: gwUser})
	wrapped := authn.NewAuthenticator(jwtAuth)

	// Build the authorizer with the same stubs the systemmcpcatalogs
	// handler tests use. Read that file before implementing — the exact
	// constructor args evolve.
	az := authz.NewAuthorizer(
		/* gatewayClient */ nil,                // see systemmcpcatalogs_test.go for the stub or fake
		/* cache kclient */ nil,
		/* uncached kclient */ nil,
		/* devMode */ false,
		/* acrHelper */ nil,
		/* skillHelper */ nil,
		/* registryNoAuth */ false,
	)
	_ = az // pass to handler middleware below; exact wiring borrowed from existing handler tests

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}/entries", func(w http.ResponseWriter, r *http.Request) {
		// Authn
		info, err := wrapped.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		// Authz — call the real Authorize path. Exact API call shape
		// depends on Authorizer's public method; see how
		// pkg/api/handlers/systemmcpcatalogs_test.go invokes it.
		if !az.Authorize(r, info) /* OR equivalent */ {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})

	return mux, issuer, cleanup, cfg, priv
}

func runWithRoles(t *testing.T, roles []string) (int, map[string]any) {
	gwUser := &gwtypes.User{ID: 42, Username: "alice", Email: "alice@example.com"}
	mux, issuer, cleanup, _, priv := buildIntegrationStack(t, gwUser)
	defer cleanup()

	rsaPriv := priv.(*testutil.RSAKey).Inner() // adjust to whatever testutil returns
	tok := testutil.MintTestJWT(t, rsaPriv, "kid-int", issuer.URL, "obot-default", "user-int",
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
	mux, _, cleanup, _, _ := buildIntegrationStack(t, nil)
	defer cleanup()
	req := httptest.NewRequest("GET", "/api/system-mcp-catalogs/default/entries", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Note for the implementer: the exact authz API surface (Authorize vs.
// AuthorizeRequest, what user.Info form it expects, what context keys
// it reads) is read straight from existing handler tests at
// pkg/api/handlers/systemmcpcatalogs_test.go and
// pkg/api/authz/authz_test.go. The above is a skeleton — the test
// stands or falls on accurately mirroring that wiring.
var _ authenticator.Request = (*oidcjwt.Authenticator)(nil) // compile-time assertion
```

- [ ] **Step 2:** `go test ./pkg/oidcjwt/... -run TestIntegration -v`
Expected: 3 PASS.

- [ ] **Step 3:** Commit.

```bash
git add pkg/oidcjwt/integration_test.go
git commit -m "test(oidcjwt): integration tests for role-based group mapping"
```

---

## Task 8: Chart values

**Files:** `chart/values.yaml`

- [ ] **Step 1:** Locate the genericOAuthAuthProvider config block.
Run: `grep -n "genericOAuth\|GENERIC_OAUTH" chart/values.yaml | head`

- [ ] **Step 2:** Add 4 keys under the existing `config:` map (or equivalent rendered env block):

```yaml
config:
  # ... existing fields ...
  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE: ""             # opt-in: empty disables the JWT authenticator
  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME: "eligible"
  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME: "roles"
  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES: "admin"
```

- [ ] **Step 3:** Render the chart if possible.
Run: `helm template chart/ | grep OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE`

- [ ] **Step 4:** Commit.

```bash
git add chart/values.yaml
git commit -m "feat(oidcjwt): chart values (audience, eligibility/roles claim names, admin-roles list)"
```

---

## Task 9: Upstream-touchpoint manifest

**Files:** `docs/studio/CHANGES.md`, `scripts/check-upstream-touchpoints.sh`

- [ ] **Step 1:** Write `docs/studio/CHANGES.md`.

```markdown
# OIDC JWT Authenticator — Upstream Touchpoints

This manifest tracks every file outside `pkg/oidcjwt/` that the OIDC JWT
authenticator integration touches. Run
`scripts/check-upstream-touchpoints.sh` after each rebase.

## Allowed touchpoints

| File | Why |
|---|---|
| `pkg/services/config.go` | Append `oidcjwt.NewAuthenticator(...)` to the authenticator union when enabled. |
| `chart/values.yaml` | Add 4 new env-var keys under the existing `config:` map. |
| `go.mod`, `go.sum` | New dependency: `github.com/coreos/go-oidc/v3`. |
| `docs/design/oidc-jwt-authn/README.md` | Design document. |
| `docs/plans/2026-06-12-oidc-jwt-authn.md` | Implementation plan. |
| `docs/studio/CHANGES.md` | This manifest. |
| `scripts/check-upstream-touchpoints.sh` | CI check. |

All other code lives under `pkg/oidcjwt/` and is purely additive.
```

- [ ] **Step 2:** Write `scripts/check-upstream-touchpoints.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE_REF="${1:-origin/main}"
ALLOWED=(
  "pkg/oidcjwt/"
  "pkg/services/config.go"
  "chart/values.yaml"
  "go.mod"
  "go.sum"
  "docs/design/oidc-jwt-authn/"
  "docs/plans/2026-06-12-oidc-jwt-authn.md"
  "docs/studio/CHANGES.md"
  "scripts/check-upstream-touchpoints.sh"
)
changed=$(git diff --name-only "$BASE_REF"...HEAD)
unexpected=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  ok=0
  for a in "${ALLOWED[@]}"; do
    if [[ "$f" == "$a"* ]]; then ok=1; break; fi
  done
  if [[ $ok -eq 0 ]]; then unexpected+=("$f"); fi
done <<< "$changed"
if [[ ${#unexpected[@]} -gt 0 ]]; then
  echo "Unexpected upstream touchpoints:"
  printf '  %s\n' "${unexpected[@]}"
  exit 1
fi
echo "OK — all changes within the allowed set."
```

```bash
chmod +x scripts/check-upstream-touchpoints.sh
```

- [ ] **Step 3:** `scripts/check-upstream-touchpoints.sh origin/main`
Expected: exits 0.

- [ ] **Step 4:** Commit.

```bash
git add docs/studio/CHANGES.md scripts/check-upstream-touchpoints.sh
git commit -m "chore(oidcjwt): upstream-touchpoint manifest + CI check"
```

---

## Task 10: Verify and push

- [ ] **Step 1:** `go test ./...`
Expected: all pass.

- [ ] **Step 2:** `go vet ./...`
Expected: clean.

- [ ] **Step 3:** `scripts/check-upstream-touchpoints.sh origin/main`
Expected: exits 0.

- [ ] **Step 4:** `git push`.

---

## Acceptance Criteria

- A JWT with `eligible: true, roles: ["admin"]` authenticates as a user with groups `[Admin, Owner, Authenticated]`, and a handler mimicking `adminAndOwnerRules` returns 200. Verified by `TestIntegration_AdminRoleReachesCatalog`.
- A JWT with `eligible: true, roles: ["user"]` authenticates as a user with groups `[Authenticated]` only, and the same handler returns 403. Verified by `TestIntegration_NonAdminForbiddenAtCatalog`.
- A JWT with `eligible: false` returns a real auth error. Verified by `TestAuthenticator_FailsWhenIneligible`.
- A non-JWT bearer, no `Authorization` header, or a JWT for a different issuer falls through unchanged. Verified by `TestAuthenticator_NoBearerFallsThrough`, `TestAuthenticator_DifferentIssuerFallsThrough`.
- A JWT for the right issuer but wrong audience is a real error. Verified by `TestVerifier_RejectsWrongAudience`.
- The upstream-touchpoint check catches any file change outside the allow-list.
- `go test ./...` and `go vet ./...` are clean.

---

## Notes for the implementer

- **No backend-principal.** Every JWT is user-subject. Group mapping is purely from the `roles` claim.

- **Generic vocabulary.** Defaults (`eligible`, `roles`, `admin`) are Obot-vocabulary. Issuers (Studio) map their internal role names to Obot vocabulary before emitting JWTs.

- **`go-oidc` owns crypto.** OIDC discovery, JWKS caching, key rotation, signature/iss/aud/exp validation — all in the library.

- **Authenticator-union semantics.** `(nil, false, nil)` = "not mine"; `(nil, false, err)` = real failure → 401. We return real errors only when the JWT is structurally ours (matching `iss`, modulo normalization) but fails validation or eligibility.

- **Identity layer contract (confirmed against current code):**
  - `pkg/gateway/types.Identity` carries `AuthProviderName`, `AuthProviderNamespace`, `ProviderUsername`, `ProviderUserID`, `ProviderIssuer`, `ProviderEmailVerified`, `Email`. **No `IssuerURL`, `Subject`, `ProviderName`, or `Username` fields.**
  - `pkg/gateway/client.Client.EnsureIdentity(ctx, id *gwtypes.Identity, timezone string) (*gwtypes.User, error)`. Third arg is a timezone string (read from `X-Obot-User-Timezone` header), not an int.
  - `ProviderUserID` format: `"iss:" + rawIssuer + "\x00sub:" + sub`. The issuer in `ProviderUserID` is **raw** (not normalized) — tests at `pkg/gateway/client/identity_generic_oauth_test.go` show fixtures with trailing slashes preserved.
  - `ProviderIssuer` = raw issuer (matches `ProviderUserID` shape).
  - `ProviderUsername` fallback: `preferred_username` → `name` → `email` → `sub` (mirrors `providers/generic-oauth-auth-provider/main.go:240-253`).
  - `AuthProviderName` is the constant `"generic-oauth-auth-provider"` (see `pkg/api/handlers/generic_oauth_validation.go:14`).
  - `AuthProviderNamespace` is `system.DefaultNamespace`. Import from `pkg/system`.

- **Issuer normalization:**
  - Trim trailing slashes when comparing JWT `iss` to config issuer (the pre-check). Mirrors `providers/generic-oauth-auth-provider/main.go:277`'s `normalizedIssuer` helper.
  - Do NOT normalize when storing on the Identity — the raw issuer is what the browser flow writes and the existing test fixtures expect.

- **Provider profile claims.** The verifier extracts `email`, `email_verified`, `preferred_username`, `name`, `picture` to match `providers/generic-oauth-auth-provider/pkg/profile/profile.go:11`. These flow through `buildIdentity` into the `*gwtypes.Identity` and `Extra` map exactly as the browser-flow `UserDecorator` (`pkg/gateway/client/auth.go:51-58`) does it.

- **Where this authenticator inserts in the union.** AFTER `client.NewUserDecorator(authenticators, gatewayClient)` (line 835 in `pkg/services/config.go`). The decorator will not rewrap our response. We are responsible for producing the final `user.Info` directly, including `UID = fmt.Sprintf("%d", gwUser.ID)` and `Name = gwUser.Username` — same pattern as `pkg/gateway/server/apikey_auth.go`.

- **`golang-jwt/jwt/v5` import.** Used only for `ParseUnverified` in the issuer pre-check.

- **No router change.** The authenticator plugs in at the union assembly site only.

- **Integration test.** The point is to assert the JWT flows through Obot's **real** `authz.NewAuthorizer` against `/api/system-mcp-catalogs/{catalog_id}/entries`, not a fake handler. Read `pkg/api/handlers/systemmcpcatalogs_test.go` to copy the stub-client wiring before writing this test.
