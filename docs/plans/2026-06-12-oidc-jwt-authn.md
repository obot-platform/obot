# OIDC JWT Authenticator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic JWT authenticator to Obot that accepts JWTs issued by the configured `generic-oauth-auth-provider`, mapping backend-principal subjects to admin/owner and user subjects to the corresponding Obot user, so the first consumer (Studio) can call Obot's existing catalog and MCP endpoints with service-identity JWTs.

**Architecture:** A new `pkg/oidcjwt/` package holds all integration code — config, a thin `go-oidc`-backed verifier wrapper, and an authenticator implementing the k8s `authenticator.Request` interface. JWT signature validation, OIDC discovery, JWKS caching, and key rotation are owned by `github.com/coreos/go-oidc/v3` (the canonical Go OIDC client used by Kubernetes, Argo CD, Vault, and others). The authenticator is appended to the existing authenticator union in `pkg/services/config.go` with one additive block. Backend-principal subjects return a `user.Info` with groups `[types.GroupAdmin, types.GroupOwner]` so the existing `adminAndOwnerRules` in `pkg/api/authz/authz.go` accept them on `/api/system-mcp-catalogs/**`, `/api/system-mcp-servers/**`, and `/api/mcp-catalogs/**` without authz changes. User-subject JWTs resolve through the existing identity layer (`pkg/gateway/client/identity.go`), creating the Obot user record on first call if absent.

**Tech Stack:** Go (same toolchain as Obot today). New dependency: `github.com/coreos/go-oidc/v3`. Existing deps reused: `github.com/golang-jwt/jwt/v5` (used only for `ParseUnverified` to read the `iss` claim before handing off to `go-oidc`). Tests use `testify`. Integration test signs JWTs with a generated RSA keypair against an `httptest.Server` that serves an OIDC discovery doc.

**Design source of truth:** `docs/design/oidc-jwt-authn/README.md` (this repo).

---

## File Structure

| Path | Status | Responsibility |
|---|---|---|
| `pkg/oidcjwt/config.go` | new | Typed config struct, env-var binding, validation. |
| `pkg/oidcjwt/config_test.go` | new | Tests for config parsing and validation. |
| `pkg/oidcjwt/verifier.go` | new | Thin wrapper around `go-oidc`'s `*oidc.Provider` + `*oidc.IDTokenVerifier`. Handles the "is this JWT ours?" pre-check (parses `iss` without verification, compares to configured issuer). |
| `pkg/oidcjwt/verifier_test.go` | new | Tests for the verifier wrapper using an in-process test issuer. |
| `pkg/oidcjwt/authenticator.go` | new | Implements `authenticator.Request`. Composes config + verifier + identity resolution. |
| `pkg/oidcjwt/authenticator_test.go` | new | Unit tests for authenticator (backend-principal path, user-subject path, fall-through cases). |
| `pkg/oidcjwt/identity_adapter.go` | new | Maps a validated user-subject JWT to an Obot user record via `pkg/gateway/client`. |
| `pkg/oidcjwt/testutil/testutil.go` | new | Shared test helpers: `NewTestIssuer`, `MintTestJWT`, `MustRSAKey`. |
| `pkg/oidcjwt/integration_test.go` | new | End-to-end test: real `authn.Authenticator`, backend-principal JWT, request hits a handler that checks groups (mimicking `adminAndOwnerRules`), expects 200. |
| `pkg/services/config.go` | modify (one block) | Load `oidcjwt.Config`, construct verifier, append `oidcjwt.NewAuthenticator(...)` to the authenticators union when enabled. |
| `chart/values.yaml` | modify | Add 3 new env keys under the existing `genericOAuthAuthProvider` block. |
| `go.mod` / `go.sum` | modify | Add `github.com/coreos/go-oidc/v3`. |
| `docs/studio/CHANGES.md` | new | Upstream-touchpoint manifest for rebase hygiene. |
| `scripts/check-upstream-touchpoints.sh` | new | CI check that flags unexpected upstream touches. |

The upstream-touch allow-list is: `pkg/services/config.go`, `chart/values.yaml`, `go.mod`, `go.sum`, plus the doc/script files above. Everything else lives under `pkg/oidcjwt/`.

---

## Task 1: Add `coreos/go-oidc` dependency and create the package skeleton

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `pkg/oidcjwt/doc.go`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/coreos/go-oidc/v3@latest`
Expected: `go.mod` and `go.sum` updated; no errors.

- [ ] **Step 2: Verify go.mod entry**

Run: `grep coreos/go-oidc go.mod`
Expected: a line like `github.com/coreos/go-oidc/v3 vX.Y.Z`.

- [ ] **Step 3: Create the package directory with a doc.go**

Path: `pkg/oidcjwt/doc.go`

```go
// Package oidcjwt implements a generic JWT authenticator that validates
// JWTs issued by the configured generic-oauth-auth-provider. See
// docs/design/oidc-jwt-authn/README.md for the contract.
package oidcjwt
```

- [ ] **Step 4: Verify the package compiles**

Run: `go build ./pkg/oidcjwt/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/oidcjwt/doc.go
git commit -m "feat(oidcjwt): scaffold package with coreos/go-oidc dep"
```

---

## Task 2: Config with env-var binding

**Files:**
- Create: `pkg/oidcjwt/config.go`
- Create: `pkg/oidcjwt/config_test.go`

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
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_BACKEND_PRINCIPAL":      "studio-deployment",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME": "studio_eligible",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.Equal(t, "https://studio.example.com/api/auth", cfg.IssuerURL)
	assert.Equal(t, "obot-default", cfg.Audience)
	assert.Equal(t, "studio-deployment", cfg.BackendPrincipal)
	assert.Equal(t, "studio_eligible", cfg.EligibilityClaimName)
	assert.True(t, cfg.Enabled())
}

func TestLoadConfigFromEnv_DefaultsAndDisabled(t *testing.T) {
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER": "https://studio.example.com/api/auth",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.False(t, cfg.Enabled())                              // empty audience -> disabled
	assert.Equal(t, "studio_eligible", cfg.EligibilityClaimName) // default
}

func envGetter(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestLoadConfigFromEnv -v`
Expected: FAIL with `undefined: LoadConfigFromEnv` and `undefined: Config`.

- [ ] **Step 3: Write minimal implementation**

Path: `pkg/oidcjwt/config.go`

```go
package oidcjwt

// Config holds the runtime configuration for the JWT authenticator. All
// fields are sourced from the existing OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*
// env-var prefix so the provider has one configuration surface.
type Config struct {
	IssuerURL            string
	Audience             string
	BackendPrincipal     string
	EligibilityClaimName string
}

const defaultEligibilityClaimName = "studio_eligible"

// Enabled reports whether the authenticator is functional. Empty audience
// or issuer means the deployment has not opted in; the authenticator is
// a no-op in the union.
func (c Config) Enabled() bool {
	return c.IssuerURL != "" && c.Audience != ""
}

// LoadConfigFromEnv reads the OBOT_GENERIC_OAUTH_AUTH_PROVIDER_* env vars
// via the supplied getter. Missing optional values fall back to defaults.
func LoadConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		IssuerURL:            getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER"),
		Audience:             getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE"),
		BackendPrincipal:     getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_BACKEND_PRINCIPAL"),
		EligibilityClaimName: getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME"),
	}
	if cfg.EligibilityClaimName == "" {
		cfg.EligibilityClaimName = defaultEligibilityClaimName
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestLoadConfigFromEnv -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/config.go pkg/oidcjwt/config_test.go
git commit -m "feat(oidcjwt): config type with env-var binding"
```

---

## Task 3: Test helpers (`pkg/oidcjwt/testutil/`)

**Files:**
- Create: `pkg/oidcjwt/testutil/testutil.go`

Both verifier tests (Task 4) and integration test (Task 8) need a test issuer + JWT minter.

- [ ] **Step 1: Write the test helpers**

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

// TestIssuer is an httptest.Server that serves an OIDC discovery doc and
// a JWKS containing one or more RSA public keys. Mutable: AddKey can be
// called after construction to simulate key rotation.
type TestIssuer struct {
	*httptest.Server
	mu   sync.Mutex
	keys map[string]*rsa.PrivateKey // kid -> priv
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

// MintTestJWT signs a JWT with the given private key + kid and standard
// iss/aud/sub/iat/exp claims. `extra` adds custom claims.
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

// MustRSAKey returns a fresh RSA-2048 key for tests.
func MustRSAKey(t *testing.T) *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}
```

- [ ] **Step 2: Verify the helper compiles**

Run: `go build ./pkg/oidcjwt/testutil/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add pkg/oidcjwt/testutil/testutil.go
git commit -m "feat(oidcjwt): test helpers (in-process OIDC issuer, JWT minter)"
```

---

## Task 4: Verifier wrapper (`pkg/oidcjwt/verifier.go`)

**Files:**
- Create: `pkg/oidcjwt/verifier.go`
- Create: `pkg/oidcjwt/verifier_test.go`

Wraps `go-oidc`'s `*oidc.Provider` + `*oidc.IDTokenVerifier`. Adds the "is this JWT ours?" pre-check: parse `iss` without verification, compare to the configured issuer. If `iss` differs, return `ErrNotMyToken` so the authenticator can fall through. If `iss` matches, hand off to `go-oidc` for full validation.

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

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "studio_eligible"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "studio-deployment", claims.Subject)
}

func TestVerifier_ReturnsNotMineForDifferentIssuer(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "studio_eligible"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X",
		"https://some-other-issuer.example.com", "obot-default", "x", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	assert.True(t, errors.Is(err, ErrNotMyToken), "expected ErrNotMyToken, got %v", err)
}

func TestVerifier_RejectsWrongAudience(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "studio_eligible"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "wrong-aud", "x", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotMyToken)) // iss matched → real failure
}

func TestVerifier_ExtractsCustomClaims(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "studio_eligible"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second,
		map[string]any{"studio_eligible": true, "email": "a@example.com", "name": "Alice"})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.True(t, claims.Eligible)
	assert.Equal(t, "a@example.com", claims.Email)
	assert.Equal(t, "Alice", claims.Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestVerifier -v`
Expected: FAIL with `undefined: NewVerifier`, `undefined: Verifier`, `undefined: ErrNotMyToken`.

- [ ] **Step 3: Write minimal implementation**

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
// authenticator (different issuer, or not parseable as a JWT). Callers
// fall through to the next authenticator in the union.
var ErrNotMyToken = errors.New("oidcjwt: token not for this authenticator")

// Claims is the validated set of claims this authenticator cares about.
type Claims struct {
	Subject  string
	Issuer   string
	Audience string
	Eligible bool
	Email    string
	Name     string
}

// Verifier wraps go-oidc's *oidc.Provider + *oidc.IDTokenVerifier with
// the fall-through semantics our authenticator needs.
type Verifier struct {
	cfg      Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

// NewVerifier constructs a Verifier. Performs OIDC discovery
// synchronously against the configured issuer; returns an error if
// discovery fails. The underlying go-oidc provider caches the JWKS and
// refreshes it on key rotation automatically.
func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
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

// Verify returns ErrNotMyToken when the raw JWT either does not parse or
// carries a different `iss` than this verifier is configured for. For
// all other validation outcomes (bad signature, wrong audience,
// expired) it returns a real error — these are "ours but invalid."
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	// Pre-check: is this JWT meant for us? Parse without verification
	// and look at iss. Avoids leaking validation errors for tokens that
	// belong to other authenticators in the union.
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
	if iss != v.cfg.IssuerURL {
		return Claims{}, ErrNotMyToken
	}

	// Full validation via go-oidc: signature + iss + aud + exp + nbf.
	idToken, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, fmt.Errorf("oidcjwt: verify: %w", err)
	}

	// Extract optional custom claims (email, name).
	var custom struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	_ = idToken.Claims(&custom)

	aud := ""
	if a, ok := mc["aud"].(string); ok {
		aud = a
	}

	return Claims{
		Subject:  idToken.Subject,
		Issuer:   idToken.Issuer,
		Audience: aud,
		Eligible: readEligibility(mc, v.cfg.EligibilityClaimName),
		Email:    custom.Email,
		Name:     custom.Name,
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestVerifier -v`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/verifier.go pkg/oidcjwt/verifier_test.go
git commit -m "feat(oidcjwt): verifier wrapper around coreos/go-oidc"
```

---

## Task 5: Authenticator — backend-principal path

**Files:**
- Create: `pkg/oidcjwt/authenticator.go`
- Create: `pkg/oidcjwt/authenticator_test.go`

- [ ] **Step 1: Write the failing test**

Path: `pkg/oidcjwt/authenticator_test.go`

```go
package oidcjwt

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticator_BackendPrincipalGrantsAdminAndOwner(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		BackendPrincipal:     "studio-deployment",
		EligibilityClaimName: "studio_eligible",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, nil)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/api/system-mcp-catalogs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.Contains(t, resp.User.GetGroups(), types.GroupOwner)
}

func TestAuthenticator_NoBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com/api/auth", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, nil, nil)

	req, _ := http.NewRequest("GET", "/", nil)
	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_DisabledConfigFallsThrough(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL /* no Audience => disabled */}
	auth := NewAuthenticator(cfg, nil, nil)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_DifferentIssuerFallsThrough(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:        issuer.URL,
		Audience:         "obot-default",
		BackendPrincipal: "studio-deployment",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, nil)

	tok := testutil.MintTestJWT(t, priv, "kid-X",
		"https://different-issuer.example.com", "obot-default", "studio-deployment", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestAuthenticator -v`
Expected: FAIL with `undefined: NewAuthenticator`, `undefined: Authenticator`.

- [ ] **Step 3: Write minimal implementation (backend-principal path only — user-subject is Task 6)**

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
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// IdentityResolver is the contract this authenticator needs to map a
// user-subject JWT to an Obot user record. Wired in Task 6 against
// pkg/gateway/client.
type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, issuer, subject string, profile UserProfile) (user.Info, error)
}

// UserProfile carries display-only claims extracted from the user-subject JWT.
type UserProfile struct {
	Email string
	Name  string
}

// Authenticator implements k8s.io/apiserver/pkg/authentication/authenticator.Request.
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
//   - (resp, true, nil) on a fully-validated, authorized JWT.
//   - (nil, false, nil) when the token does not belong to this
//     authenticator (no bearer, non-JWT, different issuer, config
//     disabled).
//   - (nil, false, err) on a real auth failure for a token that IS ours
//     but fails validation.
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

	if a.cfg.BackendPrincipal != "" && claims.Subject == a.cfg.BackendPrincipal {
		return &authenticator.Response{
			User: &user.DefaultInfo{
				UID:    "oidc-backend:" + a.cfg.BackendPrincipal,
				Name:   a.cfg.BackendPrincipal,
				Groups: []string{types.GroupAdmin, types.GroupOwner},
			},
		}, true, nil
	}

	// User-subject path implemented in Task 6.
	return nil, false, errors.New("oidcjwt: user-subject path not yet implemented")
}

func bearerToken(req *http.Request) string {
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestAuthenticator -v`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/authenticator.go pkg/oidcjwt/authenticator_test.go
git commit -m "feat(oidcjwt): authenticator with backend-principal subject path"
```

---

## Task 6: User-subject path + identity adapter

**Files:**
- Modify: `pkg/oidcjwt/authenticator.go`
- Modify: `pkg/oidcjwt/authenticator_test.go`
- Create: `pkg/oidcjwt/identity_adapter.go`

- [ ] **Step 1: Survey the identity layer**

Run: `grep -n 'EnsureIdentity\|^func .* Client' pkg/gateway/client/identity.go | head -20`
Expected: signatures for `EnsureIdentity(ctx, id *types.Identity, ...)`.

Read the current `EnsureIdentity` signature and the `types.Identity` shape in `apiclient/types/identity.go`. Adapt the adapter below to match the current API — the contract is "given `{ issuer, sub, email, name }`, look up or create the Obot user record."

- [ ] **Step 2: Write the failing user-subject tests**

Append to `pkg/oidcjwt/authenticator_test.go`:

```go
import "errors"

func TestAuthenticator_UserSubjectResolvesViaIdentity(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		BackendPrincipal:     "studio-deployment",
		EligibilityClaimName: "studio_eligible",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	stub := &stubIdentity{
		resolve: func(_ context.Context, iss, sub string, p UserProfile) (user.Info, error) {
			return &user.DefaultInfo{
				UID:    "obot-user-1",
				Name:   p.Email,
				Groups: []string{types.GroupAuthenticated},
			}, nil
		},
	}
	auth := NewAuthenticator(cfg, v, stub)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-123",
		60*time.Second, map[string]any{"studio_eligible": true, "email": "alice@example.com", "name": "Alice"})
	req, _ := http.NewRequest("GET", "/api/mcp-servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice@example.com", resp.User.GetName())
}

func TestAuthenticator_UserSubjectFailsWhenIneligible(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "studio_eligible",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, &stubIdentity{})

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-123",
		60*time.Second, map[string]any{"studio_eligible": false})
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err = auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

type stubIdentity struct {
	resolve func(ctx context.Context, iss, sub string, p UserProfile) (user.Info, error)
}

func (s *stubIdentity) ResolveOrCreate(ctx context.Context, iss, sub string, p UserProfile) (user.Info, error) {
	if s.resolve != nil {
		return s.resolve(ctx, iss, sub, p)
	}
	return nil, errors.New("stub: no resolver")
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/oidcjwt/... -run TestAuthenticator_UserSubject -v`
Expected: FAIL.

- [ ] **Step 4: Replace the placeholder in `authenticator.go`**

Replace the last paragraph of `AuthenticateRequest` (the `return nil, false, errors.New(...)` line) with:

```go
	// User-subject path.
	if !claims.Eligible {
		return nil, false, errors.New("oidcjwt: eligibility claim missing or false")
	}
	if a.identity == nil {
		return nil, false, errors.New("oidcjwt: identity resolver not configured")
	}
	info, err := a.identity.ResolveOrCreate(req.Context(), claims.Issuer, claims.Subject,
		UserProfile{Email: claims.Email, Name: claims.Name})
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: identity resolve: %w", err)
	}
	return &authenticator.Response{User: info}, true, nil
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/oidcjwt/... -v`
Expected: all tests PASS.

- [ ] **Step 6: Implement the real gateway-client adapter**

Path: `pkg/oidcjwt/identity_adapter.go`

```go
package oidcjwt

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/user"
)

// NewGatewayIdentityResolver returns an IdentityResolver backed by the
// gateway client's EnsureIdentity path — the same path the OAuth
// provider uses at browser-login time.
func NewGatewayIdentityResolver(c *gclient.Client) IdentityResolver {
	return &gatewayResolver{c: c}
}

type gatewayResolver struct{ c *gclient.Client }

func (g *gatewayResolver) ResolveOrCreate(ctx context.Context, iss, sub string, p UserProfile) (user.Info, error) {
	id := &types.Identity{
		IssuerURL:    iss,
		Subject:      sub,
		ProviderName: "generic-oauth-auth-provider",
		Email:        p.Email,
	}
	if p.Name != "" {
		id.Username = p.Name
		// id.DisplayName = p.Name  // uncomment if the field exists on current types.Identity
	}
	// EnsureIdentity signature may vary by version — adapt to the
	// current one. See pkg/gateway/client/identity.go.
	out, err := g.c.EnsureIdentity(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	return &user.DefaultInfo{
		UID:    out.Subject, // or out.ID, depending on current shape
		Name:   out.Email,
		Groups: []string{types.GroupAuthenticated},
	}, nil
}
```

> **Note:** The exact `types.Identity` field names and `EnsureIdentity` signature may have evolved. Adapt to the current shape — the contract is "look up or create the Obot user record for `{ issuer, sub }`."

- [ ] **Step 7: Verify it compiles**

Run: `go build ./pkg/oidcjwt/...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add pkg/oidcjwt/authenticator.go pkg/oidcjwt/authenticator_test.go pkg/oidcjwt/identity_adapter.go
git commit -m "feat(oidcjwt): user-subject path with gateway identity resolver"
```

---

## Task 7: Wire into the authenticator union

**Files:**
- Modify: `pkg/services/config.go`

- [ ] **Step 1: Locate the union-build region**

Run: `sed -n '825,850p' pkg/services/config.go`
Expected: the union assembly between `gserver.NewGatewayTokenReviewer` and `authn.Anonymous{}`.

- [ ] **Step 2: Insert the OIDC JWT authenticator**

Edit `pkg/services/config.go`. Right after `authenticators = union.New(authenticators, persistentTokenServer)` (around line 840), add:

```go
// OIDC JWT authenticator — accepts JWTs minted by the configured
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
    oidcResolver := oidcjwt.NewGatewayIdentityResolver(gatewayClient)
    authenticators = union.New(authenticators,
        oidcjwt.NewAuthenticator(oidcJWTCfg, oidcVerifier, oidcResolver))
}
```

Add the import `"github.com/obot-platform/obot/pkg/oidcjwt"` if not already present. `"os"` and `"fmt"` are likely already imported.

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 4: Run all package tests**

Run: `go test ./pkg/...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/services/config.go
git commit -m "feat(oidcjwt): wire into authenticator union when configured"
```

---

## Task 8: End-to-end integration test

**Files:**
- Create: `pkg/oidcjwt/integration_test.go`

- [ ] **Step 1: Write the integration test**

Path: `pkg/oidcjwt/integration_test.go`

```go
package oidcjwt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/oidcjwt"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_BackendPrincipalJWTReachesCatalog wires the
// authenticator into the same wrapper Obot uses (authn.Authenticator)
// and asserts that a backend-principal JWT reaches a handler with
// [Admin, Owner] groups — which is exactly what the existing
// adminAndOwnerRules in pkg/api/authz/authz.go require for
// /api/system-mcp-catalogs/**.
func TestIntegration_BackendPrincipalJWTReachesCatalog(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-int-1")
	defer cleanup()

	cfg := oidcjwt.Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		BackendPrincipal:     "studio-deployment",
		EligibilityClaimName: "studio_eligible",
	}
	v, err := oidcjwt.NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	jwtAuth := oidcjwt.NewAuthenticator(cfg, v, nil)
	wrapped := authn.NewAuthenticator(jwtAuth)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := wrapped.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		hasAdminOrOwner := false
		for _, g := range info.GetGroups() {
			if g == types.GroupAdmin || g == types.GroupOwner {
				hasAdminOrOwner = true
				break
			}
		}
		if !hasAdminOrOwner {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})

	tok := testutil.MintTestJWT(t, priv, "kid-int-1", issuer.URL, "obot-default",
		"studio-deployment", 60*time.Second, nil)
	req := httptest.NewRequest("GET", "/api/system-mcp-catalogs/default/entries", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out, "items")
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test ./pkg/oidcjwt/... -run TestIntegration -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/oidcjwt/integration_test.go
git commit -m "feat(oidcjwt): integration test for backend-principal JWT auth"
```

---

## Task 9: Chart values

**Files:**
- Modify: `chart/values.yaml`

- [ ] **Step 1: Locate the genericOAuthAuthProvider block**

Run: `grep -n 'genericOAuth\|GENERIC_OAUTH' chart/values.yaml | head`

- [ ] **Step 2: Add 3 new keys under that block**

```yaml
genericOAuthAuthProvider:
  # ... existing fields (issuer, clientId, clientSecret, etc.) ...
  # Service-identity JWT validation (oidcjwt). Empty audience disables
  # the JWT authenticator without affecting browser-flow login.
  audience: ""                              # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE
  backendPrincipal: ""                      # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_BACKEND_PRINCIPAL
  eligibilityClaimName: "studio_eligible"   # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME
```

If the existing block templates the issuer/clientId into env-var-prefixed names via the deployment template, add the three new env vars to the same templating.

- [ ] **Step 3: Render the chart locally if possible**

Run: `helm template chart/ > /tmp/rendered.yaml && grep -A 1 'OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE' /tmp/rendered.yaml`
Expected: the env var appears in the rendered deployment manifest.

- [ ] **Step 4: Commit**

```bash
git add chart/values.yaml
git commit -m "feat(oidcjwt): chart values for audience, backend principal, eligibility claim"
```

---

## Task 10: Upstream-touchpoint manifest

**Files:**
- Create: `docs/studio/CHANGES.md`
- Create: `scripts/check-upstream-touchpoints.sh`

- [ ] **Step 1: Write `docs/studio/CHANGES.md`**

```markdown
# OIDC JWT Authenticator — Upstream Touchpoints

This manifest tracks every file outside `pkg/oidcjwt/` that the OIDC JWT
authenticator integration touches. Run
`scripts/check-upstream-touchpoints.sh` after each rebase to verify the
list is unchanged.

## Allowed touchpoints

| File | Why |
|---|---|
| `pkg/services/config.go` | Append `oidcjwt.NewAuthenticator(...)` to the authenticator union when the config is enabled. |
| `chart/values.yaml` | Add 3 new env-var keys under the existing `genericOAuthAuthProvider` block. |
| `go.mod`, `go.sum` | New dependency: `github.com/coreos/go-oidc/v3`. |
| `docs/design/oidc-jwt-authn/README.md` | Design document. |
| `docs/plans/2026-06-12-oidc-jwt-authn.md` | Implementation plan (this file). |
| `docs/studio/CHANGES.md` | This manifest. |
| `scripts/check-upstream-touchpoints.sh` | The CI check that validates the manifest. |

All other code lives under `pkg/oidcjwt/` and is purely additive.

## When you touch a new upstream file

1. Add the row above with a one-line rationale.
2. Update the `ALLOWED` array in `scripts/check-upstream-touchpoints.sh`.
3. Commit both changes together.
```

- [ ] **Step 2: Write the check script**

Path: `scripts/check-upstream-touchpoints.sh`

```bash
#!/usr/bin/env bash
# Verifies that the oidcjwt integration touches only the files listed in
# docs/studio/CHANGES.md. Run as part of CI to catch unexpected upstream
# touches.
#
# Usage: scripts/check-upstream-touchpoints.sh [BASE_REF]
#   BASE_REF defaults to origin/main.

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
  echo
  echo "If these are intended, update docs/studio/CHANGES.md and add to ALLOWED in this script."
  exit 1
fi
echo "OK — all changes are within the allowed upstream-touchpoint set."
```

Make it executable:

```bash
chmod +x scripts/check-upstream-touchpoints.sh
```

- [ ] **Step 3: Run the script locally**

Run: `scripts/check-upstream-touchpoints.sh origin/main`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add docs/studio/CHANGES.md scripts/check-upstream-touchpoints.sh
git commit -m "chore(oidcjwt): upstream-touchpoint manifest + CI check"
```

---

## Task 11: Verify and push

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: all pass.

- [ ] **Step 2: `go vet ./...`**

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 3: Re-run the upstream-touchpoint check**

Run: `scripts/check-upstream-touchpoints.sh origin/main`
Expected: exits 0.

- [ ] **Step 4: Push the branch**

```bash
git push
```

(Branch already tracks `origin/feat/support-studio-jwt`.)

---

## Acceptance Criteria

- A backend-principal JWT signed by a test issuer authenticates as a user with groups `[Admin, Owner]`, and a handler mimicking `adminAndOwnerRules` returns 200. Verified by `TestIntegration_BackendPrincipalJWTReachesCatalog`.
- A user-subject JWT with `studio_eligible: true` resolves through the gateway identity layer to an Obot user. Verified by `TestAuthenticator_UserSubjectResolvesViaIdentity`.
- A user-subject JWT without the eligibility claim is refused with a real auth error. Verified by `TestAuthenticator_UserSubjectFailsWhenIneligible`.
- A non-JWT bearer, no `Authorization` header, or a JWT for a different issuer falls through unchanged. Verified by `TestAuthenticator_NoBearerFallsThrough`, `TestAuthenticator_DisabledConfigFallsThrough`, `TestAuthenticator_DifferentIssuerFallsThrough`.
- A JWT for the right issuer but wrong audience is a real error ("ours but invalid"). Verified by `TestVerifier_RejectsWrongAudience`.
- The check script catches any code change outside the allow-list.
- `go test ./...` and `go vet ./...` are clean.

End-to-end manual verification (after the Studio-side plan ships):

1. Bring up the Studio + Obot compose stack.
2. From Studio, mint a backend-principal JWT via the Studio-side mint helper.
3. Use `curl` from the Studio container to call Obot's `GET /api/system-mcp-catalogs/{catalog_id}/entries` with `Authorization: Bearer <jwt>`.
4. Expect 200 with the catalog payload.

---

## Notes for the implementer

- **go-oidc owns the heavy crypto.** `oidc.NewProvider(ctx, issuer)` does OIDC discovery synchronously. The returned `*oidc.Provider` caches the JWKS and refreshes it transparently on key rotation. `provider.Verifier(&oidc.Config{ClientID: aud, SupportedSigningAlgs: []string{"RS256"}})` returns a verifier that validates signature, `iss`, `aud`, `exp`, and `nbf` per RFC 7519. The only thing we add is the "is this JWT ours?" pre-check (peek at `iss` without verification) so union fall-through stays clean.

- **Authenticator-union semantics.** Each authenticator in the union returns `(response, ok, err)`. `(nil, false, nil)` means "not mine, let the next try"; `(nil, false, err)` is a real auth failure. This authenticator returns a real error only when the JWT is structurally ours (matching `iss`) but fails some other validation, or when the eligibility claim is missing on a user-subject path.

- **Identity layer compatibility.** The exact `pkg/gateway/client.Client.EnsureIdentity` signature and `types.Identity` field names may have evolved since planning. Adapt the `gatewayResolver` adapter to the current shape; the contract — "look up or create the Obot user record for `{ issuer, sub }`" — is what matters.

- **Why `golang-jwt/jwt/v5` is still imported.** Only for `ParseUnverified` in the pre-check. go-oidc's own parser is not exposed for "parse without verifying" cases. If a future version of go-oidc adds that primitive, the dependency can be dropped from `pkg/oidcjwt/` (it remains used elsewhere in Obot).

- **No router change.** The authenticator plugs in at the union assembly site only. `pkg/api/router/router.go` is untouched.
