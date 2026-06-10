# MCP Studio Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Studio (a separate identity provider) as Obot's OIDC IdP for user login AND as Obot's service-identity peer for backend calls — without ever holding a long-lived shared bearer between the two systems.

**Architecture:**
- Studio publishes a JWKS document Obot validates against. The same trust anchor authenticates both user-login ID tokens and Studio-signed service tokens. The token's `sub` claim distinguishes act-as-user from act-as-Studio-backend.
- All new Studio integration code lives in a single new top-level package `pkg/studio/`. Upstream files are touched only at three minimal patch points (config, router, helm values).
- A new image-packaged catalog manifest (`/etc/obot/studio-catalog.json` in the deployed image) is ingested at startup; Obot provisions the listed MCP servers on demand when Studio enables them.
- Service-identity API routes hang off the existing API server but are wired through the new `studio.RegisterRoutes` helper to keep the upstream router diff to one line.

**Tech Stack:** Go 1.x, golang-jwt/jwt/v5, MicahParks/jwkset (both already in `go.mod`), standard `net/http` + `http.ServeMux`, Kubernetes custom resources for User/Identity persistence (existing pattern).

**Source design:** `mcp-obot-platform/README.md` (in the Studio repo, branch `feat/better-auth-migration`).

**Sync-with-upstream principle:** This plan is structured so a future `git rebase upstream/main` should produce conflicts only at the three documented patch points listed in `docs/studio/CHANGES.md` (Task 1). All other changes are additive — new files in new directories that upstream does not modify.

---

## File Structure

### New files (all additive — zero merge surface)

| Path | Responsibility |
|---|---|
| `pkg/studio/config.go` | `StudioConfig` struct, env-tagged. Embedded into upstream `Config` at the single patch point in `pkg/services/config.go`. |
| `pkg/studio/jwks.go` | JWKS fetcher + cache for Studio's issuer URL. Refreshes on a configured interval. |
| `pkg/studio/jwt.go` | Studio JWT validator — signature (via JWKS), `iss`/`aud`/`exp`/`nbf` claim validation, returns parsed claims. |
| `pkg/studio/principal.go` | Subject resolver: maps validated `sub` to `studio-backend` principal, an existing Obot user (via Identity row lookup), or rejects. |
| `pkg/studio/middleware.go` | HTTP middleware for the service-identity routes. Validates the bearer JWT, populates request context with resolved principal. |
| `pkg/studio/identity.go` | Identity-mapping helpers: `EnsureIdentity(studioIssuer, studioSubject, claims) → (*types.User, *types.Identity)`. Lazy-creates the Obot user from claims when absent. |
| `pkg/studio/apikey.go` | Studio-managed API key store wrappers — ensure/rotate/disable/delete by `(studioIssuer, studioSubject)` tuple. Uses existing `pkg/gateway/types.APIKey` + `pkg/gateway/client`. |
| `pkg/studio/catalog.go` | Manifest loader + catalog-entry provision/teardown helpers; uses existing MCPCatalog/MCPServer Kubernetes resources. |
| `pkg/studio/handlers/apikey_handlers.go` | HTTP handlers: ensure, rotate, disable, delete. |
| `pkg/studio/handlers/catalog_handlers.go` | HTTP handlers: provision, teardown. |
| `pkg/studio/handlers/connect_handlers.go` | HTTP handler: connect-URL for configuration-required servers. |
| `pkg/studio/routes.go` | `RegisterRoutes(mux *http.ServeMux, deps RouteDeps)` — single entry point upstream calls. |
| `pkg/studio/ingester.go` | Reads the image-packaged catalog manifest at startup; ensures MCPCatalog entries exist for each. |
| `pkg/studio/testdata/` | Static fixtures: sample JWKS, signed test tokens, sample catalog manifest. |
| `pkg/studio/*_test.go` | Co-located unit tests. |
| `docs/studio/CHANGES.md` | Manifest of every upstream-file touchpoint, kept current as the plan executes. The rebase checklist. |
| `docs/studio/manifest-schema.md` | Documents the JSON shape of the image-packaged catalog manifest. |

### Upstream patch points (intentionally minimal)

| File | Change | Lines |
|---|---|---|
| `pkg/services/config.go` | Add one field: `StudioConfig studio.StudioConfig` (embedded). One new import line. | ~2 |
| `pkg/api/router/router.go` | Add one call: `studio.RegisterRoutes(mux, deps)` near the other route registrations. One new import line. | ~2 |
| `chart/values.yaml` | Append `config.OBOT_SERVER_STUDIO_*` keys in the existing `config:` block. | ~6 |

No other upstream file is modified. No upstream test is modified. No upstream type is renamed or removed.

### Open question deferred to Task 4

Whether Obot's existing user-login auth-provider model (Kubernetes `AuthProvider` CR with subprocess daemon) can be satisfied by a **generic OIDC config pointing at Studio's discovery document**, OR whether a new in-tree provider daemon is required. Task 4 is a small spike that decides — see its acceptance criteria.

---

## Task 1: Branch + manifest + plan doc landing

**Files:**
- Create: `docs/studio/CHANGES.md`
- Create: `docs/studio/manifest-schema.md` (placeholder content for Task 17 to fill in)
- This plan file is already in place at `docs/superpowers/plans/2026-06-10-mcp-studio-integration.md`

- [ ] **Step 1: Create the upstream-touchpoint manifest**

```bash
cat > docs/studio/CHANGES.md <<'EOF'
# Upstream Touchpoints for Studio Integration

This file lists every file outside `pkg/studio/`, `docs/studio/`, and
`docs/superpowers/` that the Studio integration modifies. Every entry here is
a place a future rebase against `upstream/main` may produce a conflict. The
expected resolution for each is documented.

If you add a new upstream touchpoint while implementing the plan, add it here
in the same commit.

| File | What changed | Expected conflict pattern | Resolution hint |
|---|---|---|---|
| `pkg/services/config.go` | One import + one embedded field in `Config` struct | If upstream reorders imports or adds adjacent fields, hunk may conflict | Keep our import and our field; merge ordering trivially |
| `pkg/api/router/router.go` | One import + one call to `studio.RegisterRoutes` | If upstream restructures route registration, our call may move | Re-place our call alongside the route registrations |
| `chart/values.yaml` | New `OBOT_SERVER_STUDIO_*` keys in the `config:` block | Block-level adjacency conflict possible | Keep our keys; reconcile order alphabetically |
EOF
```

- [ ] **Step 2: Create the manifest-schema placeholder**

```bash
cat > docs/studio/manifest-schema.md <<'EOF'
# Studio Catalog Manifest Schema

The image-packaged manifest at `/etc/obot/studio-catalog.json` lists the
MCP servers Obot is allowed to provision in response to Studio enablement.

Fully specified in Task 17.
EOF
```

- [ ] **Step 3: Commit**

```bash
git add docs/studio/CHANGES.md docs/studio/manifest-schema.md docs/superpowers/plans/2026-06-10-mcp-studio-integration.md
git commit -m "docs(studio): add integration plan and upstream-touchpoint manifest"
```

---

## Task 2: `pkg/studio/config.go` — StudioConfig struct

**Files:**
- Create: `pkg/studio/config.go`
- Create: `pkg/studio/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/studio/config_test.go
package studio

import (
	"os"
	"testing"
)

func TestStudioConfig_DisabledByDefault(t *testing.T) {
	for _, v := range []string{
		"OBOT_SERVER_STUDIO_ENABLED",
		"OBOT_SERVER_STUDIO_ISSUER_URL",
		"OBOT_SERVER_STUDIO_AUDIENCE",
		"OBOT_SERVER_STUDIO_BACKEND_PRINCIPAL_SUBJECT",
		"OBOT_SERVER_STUDIO_JWKS_REFRESH_INTERVAL",
	} {
		os.Unsetenv(v)
	}
	cfg := DefaultStudioConfig()
	if cfg.Enabled {
		t.Fatalf("expected Studio integration disabled by default; got enabled=true")
	}
}

func TestStudioConfig_RequiresIssuerWhenEnabled(t *testing.T) {
	cfg := StudioConfig{Enabled: true}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error when Enabled=true but IssuerURL empty")
	}
}

func TestStudioConfig_ValidatesFullyConfigured(t *testing.T) {
	cfg := StudioConfig{
		Enabled:                  true,
		IssuerURL:                "https://studio.example.com",
		Audience:                 "obot",
		BackendPrincipalSubject:  "studio-backend",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config; got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/hbanerjee/src/obot && go test ./pkg/studio/...
```

Expected: FAIL — package or symbols not defined.

- [ ] **Step 3: Implement `pkg/studio/config.go`**

```go
// pkg/studio/config.go
package studio

import (
	"errors"
	"time"
)

// StudioConfig holds all configuration for the Studio identity-provider and
// service-identity integration. Embedded into the top-level services.Config
// at exactly one patch point (pkg/services/config.go).
type StudioConfig struct {
	Enabled                 bool          `env:"OBOT_SERVER_STUDIO_ENABLED"`
	IssuerURL               string        `env:"OBOT_SERVER_STUDIO_ISSUER_URL"`
	Audience                string        `env:"OBOT_SERVER_STUDIO_AUDIENCE"`
	BackendPrincipalSubject string        `env:"OBOT_SERVER_STUDIO_BACKEND_PRINCIPAL_SUBJECT"`
	JWKSRefreshInterval     time.Duration `env:"OBOT_SERVER_STUDIO_JWKS_REFRESH_INTERVAL"`
	CatalogManifestPath     string        `env:"OBOT_SERVER_STUDIO_CATALOG_MANIFEST_PATH"`
	ClockSkewTolerance      time.Duration `env:"OBOT_SERVER_STUDIO_CLOCK_SKEW_TOLERANCE"`
}

func DefaultStudioConfig() StudioConfig {
	return StudioConfig{
		Enabled:             false,
		JWKSRefreshInterval: 5 * time.Minute,
		CatalogManifestPath: "/etc/obot/studio-catalog.json",
		ClockSkewTolerance:  30 * time.Second,
	}
}

func (c StudioConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.IssuerURL == "" {
		return errors.New("studio: IssuerURL is required when Enabled=true")
	}
	if c.Audience == "" {
		return errors.New("studio: Audience is required when Enabled=true")
	}
	if c.BackendPrincipalSubject == "" {
		return errors.New("studio: BackendPrincipalSubject is required when Enabled=true")
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/hbanerjee/src/obot && go test ./pkg/studio/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/config.go pkg/studio/config_test.go
git commit -m "feat(studio): add StudioConfig struct with env-tagged fields"
```

---

## Task 3: `pkg/studio/jwks.go` — JWKS fetcher + cache

**Files:**
- Create: `pkg/studio/jwks.go`
- Create: `pkg/studio/jwks_test.go`
- Create: `pkg/studio/testdata/jwks_sample.json` (small static JWKS fixture)

- [ ] **Step 1: Write the failing test**

```go
// pkg/studio/jwks_test.go
package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestJWKSCache_FetchOnFirstUse(t *testing.T) {
	body, err := os.ReadFile("testdata/jwks_sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	cache, err := NewJWKSCache(srv.URL, 0)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}
	keySet, err := cache.KeySet(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	if keySet == nil {
		t.Fatalf("expected non-nil keyset")
	}
}

func TestJWKSCache_RespectsRefreshInterval(t *testing.T) {
	calls := 0
	body, _ := os.ReadFile("testdata/jwks_sample.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(body)
	}))
	defer srv.Close()

	cache, _ := NewJWKSCache(srv.URL, 1*time.Hour)
	cache.KeySet(context.Background())
	cache.KeySet(context.Background())
	if calls != 1 {
		t.Fatalf("expected 1 upstream call within refresh window; got %d", calls)
	}
}
```

- [ ] **Step 2: Generate a sample JWKS fixture**

Use any standard tool (e.g., a short Go helper or an online generator restricted to test data). The fixture must contain at least one RS256 or EdDSA public key with a `kid`. Save to `pkg/studio/testdata/jwks_sample.json`. Document the matching private key (for token-signing test fixtures in Task 4) inside `testdata/README.md`.

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /Users/hbanerjee/src/obot && go test ./pkg/studio/...
```

Expected: FAIL — `NewJWKSCache` not defined.

- [ ] **Step 4: Implement `pkg/studio/jwks.go`**

Use the existing `github.com/MicahParks/jwkset` dependency. Sketch:

```go
// pkg/studio/jwks.go
package studio

import (
	"context"
	"fmt"
	"sync"
	"time"

	jwkset "github.com/MicahParks/jwkset"
)

type JWKSCache struct {
	url             string
	refreshInterval time.Duration

	mu        sync.RWMutex
	lastFetch time.Time
	storage   jwkset.Storage
}

func NewJWKSCache(jwksURL string, refreshInterval time.Duration) (*JWKSCache, error) {
	if jwksURL == "" {
		return nil, fmt.Errorf("studio: jwks URL required")
	}
	return &JWKSCache{
		url:             jwksURL,
		refreshInterval: refreshInterval,
	}, nil
}

// KeySet returns a jwkset.Storage that the JWT validator can use to look up
// signing keys by `kid`. Refreshes lazily on first use and after
// refreshInterval has elapsed.
func (c *JWKSCache) KeySet(ctx context.Context) (jwkset.Storage, error) {
	c.mu.RLock()
	fresh := c.storage != nil && time.Since(c.lastFetch) < c.refreshInterval
	c.mu.RUnlock()
	if fresh {
		return c.storage, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-checked locking
	if c.storage != nil && time.Since(c.lastFetch) < c.refreshInterval {
		return c.storage, nil
	}
	storage, err := jwkset.NewDefaultHTTPClientCtx(ctx, []string{c.url})
	if err != nil {
		return nil, fmt.Errorf("studio: fetch JWKS: %w", err)
	}
	c.storage = storage
	c.lastFetch = time.Now()
	return c.storage, nil
}
```

> **Implementer note:** verify `jwkset.NewDefaultHTTPClientCtx` signature against the pinned version in `go.mod` — the package's API has shifted between minor versions. Adjust the call accordingly. If the helper does not exist, instantiate `jwkset.NewMemoryStorage` and populate it from a manual HTTP fetch.

- [ ] **Step 5: Run tests**

```bash
cd /Users/hbanerjee/src/obot && go test ./pkg/studio/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/studio/jwks.go pkg/studio/jwks_test.go pkg/studio/testdata/jwks_sample.json pkg/studio/testdata/README.md
git commit -m "feat(studio): add JWKS fetcher with refresh-interval cache"
```

---

## Task 4: User-login provider spike — decide path

**Files:**
- Create: `docs/studio/decisions/2026-06-10-user-login-provider.md`

This is an investigative task, not a code task. Output is a decision document committed before any user-login code is written.

- [ ] **Step 1: Read the existing auth-provider plumbing**

Read these files end-to-end and note the contract:
- `pkg/api/handlers/authprovider.go`
- `pkg/api/handlers/providers/authproviders.go`
- `pkg/gateway/server/dispatcher/dispatcher.go`
- `apiclient/types/authprovider.go`

- [ ] **Step 2: Check whether a generic OIDC provider already exists**

```bash
cd /Users/hbanerjee/src/obot && grep -rn "generic\|openid\|oidc" --include="*.go" pkg/api/handlers/providers/ pkg/gateway/server/dispatcher/ 2>/dev/null
```

- [ ] **Step 3: Write the decision doc**

The doc answers three questions:
1. Can a Studio-capable user-login provider be expressed as configuration on the existing AuthProvider CR (with `Spec.Command` pointing at an existing generic-OIDC daemon)? If yes, no new Go binary is needed — only an `AuthProvider` manifest documenting the Studio configuration.
2. If no, what is the smallest new binary (path, package layout, behavior contract) that satisfies the AuthProvider command/args interface AND speaks OIDC against Studio's issuer?
3. Where does that new binary live? `cmd/studio-auth-provider/main.go` is the proposed location if Obot uses `cmd/<name>/main.go` for its multi-binary builds; otherwise mirror the upstream convention exactly.

The decision doc commits to one of (A) "no new binary — configure existing generic OIDC" or (B) "new binary at `<exact path>`". All subsequent user-login tasks branch on this decision.

- [ ] **Step 4: Commit**

```bash
git add docs/studio/decisions/2026-06-10-user-login-provider.md
git commit -m "docs(studio): decide user-login provider path"
```

---

## Task 5: `pkg/studio/jwt.go` — service-token validator

**Files:**
- Create: `pkg/studio/jwt.go`
- Create: `pkg/studio/jwt_test.go`
- Add: `pkg/studio/testdata/sign_token.go` (test-only helper that signs tokens with the fixture private key)

- [ ] **Step 1: Write the failing test**

```go
// pkg/studio/jwt_test.go
package studio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func newTestValidator(t *testing.T) (*JWTValidator, func()) {
	t.Helper()
	jwksBody, _ := os.ReadFile("testdata/jwks_sample.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksBody)
	}))
	cache, _ := NewJWKSCache(srv.URL, time.Hour)
	cfg := StudioConfig{
		Enabled:                 true,
		IssuerURL:               "https://studio.test",
		Audience:                "obot",
		BackendPrincipalSubject: "studio-backend",
		ClockSkewTolerance:      30 * time.Second,
	}
	return NewJWTValidator(cfg, cache), srv.Close
}

func TestJWTValidator_AcceptsBackendPrincipalToken(t *testing.T) {
	v, cleanup := newTestValidator(t)
	defer cleanup()
	token := SignTestToken(t, "https://studio.test", "studio-backend", "obot", time.Now().Add(1*time.Minute))
	claims, err := v.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Subject != "studio-backend" {
		t.Fatalf("expected sub=studio-backend; got %q", claims.Subject)
	}
}

func TestJWTValidator_RejectsWrongIssuer(t *testing.T) {
	v, cleanup := newTestValidator(t)
	defer cleanup()
	token := SignTestToken(t, "https://attacker.example", "studio-backend", "obot", time.Now().Add(1*time.Minute))
	if _, err := v.Validate(context.Background(), token); err == nil {
		t.Fatalf("expected rejection on wrong issuer")
	}
}

func TestJWTValidator_RejectsWrongAudience(t *testing.T) {
	v, cleanup := newTestValidator(t)
	defer cleanup()
	token := SignTestToken(t, "https://studio.test", "studio-backend", "not-obot", time.Now().Add(1*time.Minute))
	if _, err := v.Validate(context.Background(), token); err == nil {
		t.Fatalf("expected rejection on wrong audience")
	}
}

func TestJWTValidator_RejectsExpired(t *testing.T) {
	v, cleanup := newTestValidator(t)
	defer cleanup()
	token := SignTestToken(t, "https://studio.test", "studio-backend", "obot", time.Now().Add(-1*time.Hour))
	if _, err := v.Validate(context.Background(), token); err == nil {
		t.Fatalf("expected rejection on expired token")
	}
}
```

- [ ] **Step 2: Implement the test-signing helper**

`pkg/studio/testdata/sign_token.go` is a test-only file that uses the private key paired with `jwks_sample.json` (the pairing is documented in `testdata/README.md`, Task 3). Build tag: `//go:build testdata`. Export `SignTestToken(t, iss, sub, aud, exp) string`.

- [ ] **Step 3: Implement `pkg/studio/jwt.go`**

```go
// pkg/studio/jwt.go
package studio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type StudioClaims struct {
	jwt.RegisteredClaims
	// Add any Studio-specific claims here later (e.g. role/eligibility).
}

type JWTValidator struct {
	cfg   StudioConfig
	jwks  *JWKSCache
}

func NewJWTValidator(cfg StudioConfig, jwks *JWKSCache) *JWTValidator {
	return &JWTValidator{cfg: cfg, jwks: jwks}
}

func (v *JWTValidator) Validate(ctx context.Context, raw string) (*StudioClaims, error) {
	storage, err := v.jwks.KeySet(ctx)
	if err != nil {
		return nil, fmt.Errorf("studio: jwks: %w", err)
	}
	claims := &StudioClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("studio: token missing kid")
		}
		jwk, err := storage.KeyRead(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("studio: unknown kid %q: %w", kid, err)
		}
		return jwk.Marshal().Key, nil
	},
		jwt.WithIssuer(v.cfg.IssuerURL),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithLeeway(v.cfg.ClockSkewTolerance),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("studio: token not valid")
	}
	_ = time.Now() // placate unused-import linters in some builds
	return claims, nil
}
```

> **Implementer note:** verify the exact `jwkset` API for "key by kid" in the pinned version. The marshal call to extract the underlying `crypto.PublicKey` may differ — check the godoc of the imported version. The flow (storage.KeyRead → public key → jwt.ParseWithClaims callback) is the structural intent.

- [ ] **Step 4: Run tests**

```bash
cd /Users/hbanerjee/src/obot && go test -tags=testdata ./pkg/studio/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/jwt.go pkg/studio/jwt_test.go pkg/studio/testdata/sign_token.go pkg/studio/testdata/README.md
git commit -m "feat(studio): add JWT validator for Studio-signed tokens"
```

---

## Task 6: `pkg/studio/principal.go` — subject resolver

**Files:**
- Create: `pkg/studio/principal.go`
- Create: `pkg/studio/principal_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/studio/principal_test.go
package studio

import "testing"

func TestResolvePrincipal_StudioBackend(t *testing.T) {
	cfg := StudioConfig{BackendPrincipalSubject: "studio-backend"}
	p := ResolvePrincipal(cfg, "studio-backend")
	if !p.IsStudioBackend() {
		t.Fatalf("expected backend principal; got user principal")
	}
}

func TestResolvePrincipal_User(t *testing.T) {
	cfg := StudioConfig{BackendPrincipalSubject: "studio-backend"}
	p := ResolvePrincipal(cfg, "studio-user-abc")
	if p.IsStudioBackend() {
		t.Fatalf("expected user principal; got backend")
	}
	if p.StudioSubject != "studio-user-abc" {
		t.Fatalf("subject not preserved")
	}
}
```

- [ ] **Step 2: Implement `pkg/studio/principal.go`**

```go
// pkg/studio/principal.go
package studio

type Principal struct {
	StudioIssuer  string
	StudioSubject string
	isBackend     bool
}

func (p Principal) IsStudioBackend() bool { return p.isBackend }
func (p Principal) IsStudioUser() bool    { return !p.isBackend }

func ResolvePrincipal(cfg StudioConfig, subject string) Principal {
	return Principal{
		StudioIssuer:  cfg.IssuerURL,
		StudioSubject: subject,
		isBackend:     subject == cfg.BackendPrincipalSubject,
	}
}
```

- [ ] **Step 3: Run tests and commit**

```bash
cd /Users/hbanerjee/src/obot && go test ./pkg/studio/...
git add pkg/studio/principal.go pkg/studio/principal_test.go
git commit -m "feat(studio): add subject resolver for backend-vs-user principals"
```

---

## Task 7: `pkg/studio/middleware.go` — JWT-validating HTTP middleware

**Files:**
- Create: `pkg/studio/middleware.go`
- Create: `pkg/studio/middleware_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/studio/middleware_test.go
package studio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMiddleware_AcceptsValidBackendToken(t *testing.T) {
	jwksBody, _ := os.ReadFile("testdata/jwks_sample.json")
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksBody)
	}))
	defer jwksServer.Close()

	cfg := StudioConfig{
		Enabled:                 true,
		IssuerURL:               "https://studio.test",
		Audience:                "obot",
		BackendPrincipalSubject: "studio-backend",
		ClockSkewTolerance:      30 * time.Second,
	}
	cache, _ := NewJWKSCache(jwksServer.URL, time.Hour)
	validator := NewJWTValidator(cfg, cache)

	var seen Principal
	handler := RequireBackendPrincipal(cfg, validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	token := SignTestToken(t, "https://studio.test", "studio-backend", "obot", time.Now().Add(1*time.Minute))
	req := httptest.NewRequest("GET", "/test", nil).WithContext(context.Background())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d", rec.Code)
	}
	if !seen.IsStudioBackend() {
		t.Fatalf("expected backend principal in context")
	}
}

func TestMiddleware_Rejects401WhenMissingAuth(t *testing.T) {
	cfg := StudioConfig{Enabled: true}
	handler := RequireBackendPrincipal(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401; got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Implement `pkg/studio/middleware.go`**

```go
// pkg/studio/middleware.go
package studio

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const principalCtxKey ctxKey = "studio.principal"

func PrincipalFromContext(ctx context.Context) Principal {
	if v, ok := ctx.Value(principalCtxKey).(Principal); ok {
		return v
	}
	return Principal{}
}

// RequireBackendPrincipal validates the bearer token and ensures the resolved
// principal is the configured Studio backend. Used by service-identity routes
// that act on any user's behalf.
func RequireBackendPrincipal(cfg StudioConfig, validator *JWTValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := validator.Validate(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			principal := ResolvePrincipal(cfg, claims.Subject)
			if !principal.IsStudioBackend() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), principalCtxKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
```

- [ ] **Step 3: Run tests and commit**

```bash
cd /Users/hbanerjee/src/obot && go test -tags=testdata ./pkg/studio/...
git add pkg/studio/middleware.go pkg/studio/middleware_test.go
git commit -m "feat(studio): add JWT-validating middleware for service-identity routes"
```

---

## Task 8: `pkg/studio/identity.go` — identity-mapping helpers

**Files:**
- Create: `pkg/studio/identity.go`
- Create: `pkg/studio/identity_test.go`

Bridges Studio's `{IssuerURL, Subject}` tuple to the existing Obot `Identity` table. Lazy-creates the User row from request claims when the Studio user has never logged in before.

The exact storage calls depend on the gateway client API. The implementer should adapt the function signatures below to the actual `pkg/gateway/client/...` surface. The behavior contract is:

- `EnsureIdentity(ctx, store, claims) → (*User, *Identity, error)`
- Looks up Identity by `(AuthProviderName="studio", AuthProviderNamespace=<configured ns>, ProviderUserID=claims.Subject)`.
- If found: returns the existing User + Identity.
- If not found: creates a new User from `claims.Email`, `claims.Name`, plus a new Identity row.
- Idempotent across replicas — uses the database's unique constraint on `(AuthProviderName, AuthProviderNamespace, HashedProviderUserID)`.

- [ ] **Step 1: Read the existing identity-management code**

```bash
cd /Users/hbanerjee/src/obot && cat pkg/gateway/client/identity*.go pkg/gateway/types/identity.go pkg/gateway/types/users.go 2>&1 | head -200
```

Document the relevant call signatures in a comment at the top of `identity.go` so the unit tests can mock the right interface.

- [ ] **Step 2: Write the test and implementation against an interface**

Define an `IdentityStore` interface in `identity.go` that captures only the methods Studio integration needs. The test uses a fake implementation. The production wiring (Task 14) provides a real implementation backed by `pkg/gateway/client`.

```go
// pkg/studio/identity.go (sketch)
package studio

import "context"

type IdentityStore interface {
	FindByProviderUserID(ctx context.Context, providerName, providerNamespace, providerUserID string) (*UserRecord, *IdentityRecord, error)
	CreateUserAndIdentity(ctx context.Context, in NewUserInput) (*UserRecord, *IdentityRecord, error)
}

type UserRecord struct {
	ID          uint
	DisplayName string
	Username    string
	Email       string
}

type IdentityRecord struct {
	ProviderName      string
	ProviderNamespace string
	ProviderUserID    string
	UserID            uint
}

type NewUserInput struct {
	StudioIssuer        string
	StudioSubject       string
	Email               string
	DisplayName         string
	ProviderNamespace   string
}

func EnsureIdentity(ctx context.Context, store IdentityStore, claims StudioClaims, providerNamespace string) (*UserRecord, *IdentityRecord, error) {
	user, identity, err := store.FindByProviderUserID(ctx, "studio", providerNamespace, claims.Subject)
	if err == nil && identity != nil {
		return user, identity, nil
	}
	return store.CreateUserAndIdentity(ctx, NewUserInput{
		StudioIssuer:      claims.Issuer,
		StudioSubject:     claims.Subject,
		ProviderNamespace: providerNamespace,
		// Email + DisplayName: pull from custom claims if added later.
	})
}
```

Write tests with a fake store that asserts the "find then create" behavior.

- [ ] **Step 3: Commit**

```bash
git add pkg/studio/identity.go pkg/studio/identity_test.go
git commit -m "feat(studio): add identity mapping for {issuer,subject} -> Obot user"
```

---

## Task 9: `pkg/studio/apikey.go` — Studio-managed API key wrappers

**Files:**
- Create: `pkg/studio/apikey.go`
- Create: `pkg/studio/apikey_test.go`

Wraps the existing `pkg/gateway/server/apikey.go` storage so Studio's ensure/rotate/disable/delete contract maps onto it. Idempotency by `(studioIssuer, studioSubject, "studio-managed")` tag.

Acceptance criteria (the tests):

1. `EnsureKey({issuer, subject, scope: [serverIds], expiresAt}, store)`:
   - If no Studio-managed key exists for that subject, creates one with the configured scope (stored in `APIKey.MCPServerIDs`) and the requested expiry, returns the plaintext once.
   - If one exists with the same scope and a non-expired expiry: returns metadata only (no plaintext).
   - If one exists with a different scope or expiring soon: rotates and returns the new plaintext.

2. `RotateKey({issuer, subject, scope, expiresAt})`: invalidates the existing key, creates a new one with the new scope/expiry, returns the new plaintext.

3. `DisableKey({issuer, subject, keyID})`: marks the key as disabled; subsequent validation returns "disabled."

4. `DeleteKey({issuer, subject, keyID})`: removes the key. Idempotent — returns success if the key is already absent.

Implement against an `APIKeyStore` interface (analogous to Task 8's `IdentityStore`) so tests can use a fake.

- [ ] **Step 1: Read upstream apikey storage**

```bash
cd /Users/hbanerjee/src/obot && cat pkg/gateway/server/apikey.go pkg/gateway/client/apikey.go pkg/gateway/types/apikeys.go 2>&1 | head -200
```

- [ ] **Step 2: Write tests against the interface (TDD)**

Tests assert each of the four acceptance criteria above with a fake `APIKeyStore`.

- [ ] **Step 3: Implement**

Use a name convention for Studio-managed keys so they don't collide with user-created keys (e.g., `Name = "studio-managed:" + studioSubject`).

- [ ] **Step 4: Commit**

```bash
git add pkg/studio/apikey.go pkg/studio/apikey_test.go
git commit -m "feat(studio): add Studio-managed API key lifecycle wrappers"
```

---

## Task 10: `pkg/studio/handlers/apikey_handlers.go` — HTTP handlers

**Files:**
- Create: `pkg/studio/handlers/apikey_handlers.go`
- Create: `pkg/studio/handlers/apikey_handlers_test.go`

The four routes:

| Method + Path | Handler |
|---|---|
| `PUT /api/studio/users/{studioSubject}/api-key` | `EnsureKey` |
| `POST /api/studio/users/{studioSubject}/api-key/rotate` | `RotateKey` |
| `POST /api/studio/users/{studioSubject}/api-key/{keyId}/disable` | `DisableKey` |
| `DELETE /api/studio/users/{studioSubject}/api-key/{keyId}` | `DeleteKey` |

Request bodies and response shapes follow the design doc's "Obot endpoint contract" table. Use `apiclient/types`-style error envelopes consistent with upstream.

Each handler:
1. Reads the `studioSubject` from the path.
2. Calls into `pkg/studio/identity.EnsureIdentity` (for ensure) or assumes Identity exists (for rotate/disable/delete; returns 404 otherwise).
3. Calls into `pkg/studio/apikey` for the lifecycle operation.
4. Returns the result.

The middleware from Task 7 (`RequireBackendPrincipal`) is applied at route registration in Task 14; handlers themselves do not re-validate the bearer token.

- [ ] **Step 1: TDD each handler (4 sub-tasks)**

For each route:
1. Write a test that calls the handler with `httptest.NewRequest` + a context carrying a backend principal.
2. Run, observe failure.
3. Implement.
4. Run, observe pass.

- [ ] **Step 2: Commit (one commit per route, four commits total)**

```bash
git commit -m "feat(studio): add ensure-key HTTP handler"
# ... rotate, disable, delete
```

---

## Task 11: `pkg/studio/catalog.go` + `pkg/studio/handlers/catalog_handlers.go`

**Files:**
- Create: `pkg/studio/catalog.go`
- Create: `pkg/studio/catalog_test.go`
- Create: `pkg/studio/handlers/catalog_handlers.go`
- Create: `pkg/studio/handlers/catalog_handlers_test.go`

The two routes:

| Method + Path | Handler |
|---|---|
| `POST /api/studio/catalog/{serverId}/provision` | `ProvisionCatalogEntry` |
| `POST /api/studio/catalog/{serverId}/teardown` | `TeardownCatalogEntry` |

`catalog.go` bridges to the existing MCPCatalog/MCPServer Kubernetes resources. Provision creates or activates the MCPServer for that serverId from the manifest's definition; teardown deactivates it.

Acceptance criteria:
- Provision is idempotent — calling twice yields the same MCPServer in the same state.
- Provision returns 404 if `serverId` is not in the loaded manifest (the manifest is the gate, not Obot's full catalog).
- Teardown is idempotent — calling on a non-existent or already-torn-down MCPServer returns success.

- [ ] **Step 1: Read MCPServer/MCPCatalog storage code**

```bash
cd /Users/hbanerjee/src/obot && find pkg/storage -name "*mcp*" -type f 2>&1 | head
cd /Users/hbanerjee/src/obot && grep -rn "MCPServer\|MCPCatalog" --include="*.go" pkg/api/handlers/mcp*.go pkg/services 2>&1 | head -30
```

- [ ] **Step 2: TDD as in earlier tasks**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(studio): add catalog provision/teardown helpers and HTTP handlers"
```

---

## Task 12: `pkg/studio/handlers/connect_handlers.go` — connect-URL

**Files:**
- Create: `pkg/studio/handlers/connect_handlers.go`
- Create: `pkg/studio/handlers/connect_handlers_test.go`

The route:

| Method + Path | Handler |
|---|---|
| `GET /api/studio/users/{studioSubject}/mcp-servers/{serverId}/connect-url` | `GetConnectURL` |

Returns the URL Studio should redirect the user to for an OAuth or configuration flow on that MCPServer.

Implementation: read the existing `/api/mcp-servers/{id}/check-oauth` flow (referenced in router.go:132) and the per-server connect URL mechanism. The handler resolves the Studio subject to an Obot user, then composes the connect URL using the same mechanism the existing Obot UI uses — exposing it through a stable URL so Studio doesn't depend on UI internals.

- [ ] **Step 1: Read the existing connect-URL machinery**

```bash
cd /Users/hbanerjee/src/obot && grep -rn "check-oauth\|connect-url\|CheckOAuth\|ConnectURL" --include="*.go" pkg/ 2>&1 | head
```

- [ ] **Step 2: TDD as before**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(studio): add stable connect-URL contract for MCP servers"
```

---

## Task 13: `pkg/studio/ingester.go` — manifest loader + startup hook

**Files:**
- Create: `pkg/studio/ingester.go`
- Create: `pkg/studio/ingester_test.go`
- Update: `docs/studio/manifest-schema.md` with the final JSON schema

Acceptance criteria:
- Reads `cfg.CatalogManifestPath` at startup; if absent and `Enabled=false`, no-op. If `Enabled=true` and the file is missing, return an error.
- Validates each manifest entry against the schema (server id, display name, description, requires-config flag, anything else required by Obot's MCPCatalog).
- For each entry, ensures a corresponding MCPCatalog "supported" entry exists. Idempotent.
- Returns a non-nil error on schema violation; logs and continues on transient backend errors (so a flaky etcd doesn't crash startup).

The schema:

```json
{
  "version": 1,
  "servers": [
    {
      "id": "github",
      "displayName": "GitHub",
      "description": "Read and write GitHub repos via the MCP server.",
      "requiresPerUserOAuth": true
    }
  ]
}
```

- [ ] **Step 1: TDD with table-driven tests**

- [ ] **Step 2: Commit**

```bash
git commit -m "feat(studio): add image-packaged catalog manifest ingester"
```

---

## Task 14: `pkg/studio/routes.go` — single route-registration entry point

**Files:**
- Create: `pkg/studio/routes.go`
- Create: `pkg/studio/routes_test.go`

This file is the **only thing upstream calls** to wire the Studio integration. The upstream patch in Task 15 is a single function call.

```go
// pkg/studio/routes.go
package studio

import (
	"context"
	"log/slog"
	"net/http"
)

type RouteDeps struct {
	Config        StudioConfig
	IdentityStore IdentityStore
	APIKeyStore   APIKeyStore
	CatalogStore  CatalogStore
	Logger        *slog.Logger
}

// RegisterRoutes wires every Studio integration route into the provided mux.
// No-op when StudioConfig.Enabled is false.
func RegisterRoutes(ctx context.Context, mux *http.ServeMux, deps RouteDeps) error {
	if !deps.Config.Enabled {
		return nil
	}
	if err := deps.Config.Validate(); err != nil {
		return err
	}
	jwks, err := NewJWKSCache(deps.Config.IssuerURL+"/.well-known/jwks.json", deps.Config.JWKSRefreshInterval)
	if err != nil {
		return err
	}
	validator := NewJWTValidator(deps.Config, jwks)
	requireBackend := RequireBackendPrincipal(deps.Config, validator)

	// API key lifecycle
	mux.Handle("PUT /api/studio/users/{studioSubject}/api-key", requireBackend(handlers.NewEnsureKey(deps)))
	mux.Handle("POST /api/studio/users/{studioSubject}/api-key/rotate", requireBackend(handlers.NewRotateKey(deps)))
	mux.Handle("POST /api/studio/users/{studioSubject}/api-key/{keyId}/disable", requireBackend(handlers.NewDisableKey(deps)))
	mux.Handle("DELETE /api/studio/users/{studioSubject}/api-key/{keyId}", requireBackend(handlers.NewDeleteKey(deps)))

	// Catalog
	mux.Handle("POST /api/studio/catalog/{serverId}/provision", requireBackend(handlers.NewProvisionCatalog(deps)))
	mux.Handle("POST /api/studio/catalog/{serverId}/teardown", requireBackend(handlers.NewTeardownCatalog(deps)))

	// Connect URL
	mux.Handle("GET /api/studio/users/{studioSubject}/mcp-servers/{serverId}/connect-url", requireBackend(handlers.NewGetConnectURL(deps)))

	return nil
}
```

- [ ] **Step 1: Test that no routes register when Enabled=false**

- [ ] **Step 2: Test that all seven routes register when Enabled=true with valid config**

- [ ] **Step 3: Test that an invalid config returns an error and registers nothing**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(studio): add RegisterRoutes single entry point"
```

---

## Task 15: Wire `RegisterRoutes` into upstream router (PATCH POINT 1)

**Files:**
- Modify: `pkg/api/router/router.go` (one new import + one new call)

This is one of the three upstream touchpoints. Update `docs/studio/CHANGES.md` in the same commit if the exact line/context shifts.

- [ ] **Step 1: Identify the route-registration block**

```bash
cd /Users/hbanerjee/src/obot && grep -n "mux.HandleFunc\|mux.Handle" pkg/api/router/router.go | head -10
```

- [ ] **Step 2: Add the import and the call**

Pseudo-diff:

```diff
 import (
   ...
+  "github.com/obot-platform/obot/pkg/studio"
 )
 ...
 func Router(...) {
   ...
   // existing route registrations
+  if err := studio.RegisterRoutes(ctx, mux, studio.RouteDeps{
+    Config:        cfg.StudioConfig,
+    IdentityStore: studio.NewIdentityStoreFromGateway(gatewayClient),
+    APIKeyStore:   studio.NewAPIKeyStoreFromGateway(gatewayClient),
+    CatalogStore:  studio.NewCatalogStoreFromKclient(kclient),
+    Logger:        logger,
+  }); err != nil {
+    return fmt.Errorf("register Studio routes: %w", err)
+  }
   ...
 }
```

The `NewXxxFromYyy` adapters are thin constructors that satisfy the Task 8/9/11 interfaces. They live in `pkg/studio/adapters.go` (add this file alongside the others; not separately listed in File Structure because it is mechanical glue).

- [ ] **Step 3: Run `go build ./...` to verify the wiring compiles**

```bash
cd /Users/hbanerjee/src/obot && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add pkg/api/router/router.go pkg/studio/adapters.go docs/studio/CHANGES.md
git commit -m "feat(studio): wire RegisterRoutes into upstream router"
```

---

## Task 16: Embed StudioConfig in services.Config (PATCH POINT 2)

**Files:**
- Modify: `pkg/services/config.go` (one new import + one new field)

- [ ] **Step 1: Add the field**

Pseudo-diff:

```diff
 import (
   ...
+  "github.com/obot-platform/obot/pkg/studio"
 )

 type Config struct {
   ...
+  StudioConfig studio.StudioConfig
 }
```

- [ ] **Step 2: Apply defaults from `DefaultStudioConfig`**

If `pkg/services/config.go` has a `NewConfig` or default-constructor, splice in `DefaultStudioConfig()`. If env vars are loaded by struct tag automatically (e.g. via `envconfig`), no further change is needed — the field's env tags pick up the variables.

- [ ] **Step 3: Build + commit**

```bash
cd /Users/hbanerjee/src/obot && go build ./...
git add pkg/services/config.go docs/studio/CHANGES.md
git commit -m "feat(studio): embed StudioConfig in services.Config"
```

---

## Task 17: Helm values (PATCH POINT 3)

**Files:**
- Modify: `chart/values.yaml`

- [ ] **Step 1: Append config keys**

In the existing `config:` block of `chart/values.yaml`:

```yaml
config:
  # ...existing keys...
  OBOT_SERVER_STUDIO_ENABLED: "false"
  OBOT_SERVER_STUDIO_ISSUER_URL: ""
  OBOT_SERVER_STUDIO_AUDIENCE: ""
  OBOT_SERVER_STUDIO_BACKEND_PRINCIPAL_SUBJECT: ""
  OBOT_SERVER_STUDIO_JWKS_REFRESH_INTERVAL: "5m"
  OBOT_SERVER_STUDIO_CATALOG_MANIFEST_PATH: "/etc/obot/studio-catalog.json"
  OBOT_SERVER_STUDIO_CLOCK_SKEW_TOLERANCE: "30s"
```

- [ ] **Step 2: Verify the helm chart still renders**

```bash
cd /Users/hbanerjee/src/obot && helm template chart/ > /dev/null
```

- [ ] **Step 3: Commit**

```bash
git add chart/values.yaml docs/studio/CHANGES.md
git commit -m "feat(studio): add Studio integration env vars to helm chart"
```

---

## Task 18: Ingester startup hook

**Files:**
- Modify or create: a startup hook point. Identify during Task 4 — likely `pkg/services/services.go` or wherever the long-running Obot service constructor runs.

If the integration point IS in an existing file (i.e., a fourth upstream touchpoint), add it to `docs/studio/CHANGES.md`. If it can be done from inside `pkg/studio/` (e.g. by hooking off `RegisterRoutes`), prefer that.

Acceptance criterion: when `Enabled=true`, the ingester runs once on process start and the result is observable through logs (`slog.Info("studio: ingested N catalog entries")`).

- [ ] **Step 1: Identify the smallest viable hook**

- [ ] **Step 2: Wire ingester invocation**

- [ ] **Step 3: Commit**

```bash
git add ...
git commit -m "feat(studio): run catalog manifest ingester at startup"
```

---

## Task 19: User-login provider implementation

**Branching on Task 4's decision:**

- **If Path A (configure existing generic OIDC)**: write an `AuthProvider` manifest under `docs/studio/manifests/studio-auth-provider.yaml` that operators apply. Add a section to `docs/studio/CHANGES.md` linking the manifest.

- **If Path B (new in-tree binary)**: implement `cmd/studio-auth-provider/main.go` (or wherever Task 4 decided), build it into the container image, and wire it into the Obot deployment. This is a substantial sub-plan — when reached, the implementer should break it into bite-sized sub-tasks following the same TDD pattern.

This task is intentionally left at higher granularity because its shape depends entirely on Task 4's outcome.

- [ ] **Step 1: Confirm decision from Task 4**

- [ ] **Step 2: Implement the chosen path**

- [ ] **Step 3: End-to-end verification — a browser-driven OIDC login against a Studio test instance issues an Obot session**

- [ ] **Step 4: Commit (multiple commits, one per sub-task)**

---

## Task 20: End-to-end integration test

**Files:**
- Create: `pkg/studio/integration_test.go` (build tag `//go:build integration`)

A test that:
1. Stands up an in-process Studio JWKS HTTP server.
2. Stands up the Obot HTTP API server with `Enabled=true` and pointing at the fake JWKS.
3. Mints a Studio-signed token (backend principal).
4. Calls each service-identity route end-to-end (ensure key, rotate, disable, delete, provision catalog, teardown, connect URL).
5. Asserts persistence of side effects via the gateway client.

- [ ] **Step 1: Scaffold the test**
- [ ] **Step 2: Wire it as a CI lane (separate workflow that only runs on `integration` tag)**
- [ ] **Step 3: Commit**

```bash
git commit -m "test(studio): add end-to-end integration test for service-identity API"
```

---

## Task 21: Sync-check tooling

**Files:**
- Create: `scripts/check-upstream-touchpoints.sh`

A script the team runs after every `git fetch upstream && git rebase upstream/main` that:
1. Lists every file modified between `upstream/main..HEAD`.
2. Compares against the manifest in `docs/studio/CHANGES.md`.
3. Flags any unexpected upstream-file modification.

- [ ] **Step 1: Write the script**

```bash
#!/usr/bin/env bash
# scripts/check-upstream-touchpoints.sh
set -euo pipefail

expected_touchpoints=(
  "pkg/services/config.go"
  "pkg/api/router/router.go"
  "chart/values.yaml"
)

modified=$(git diff --name-only upstream/main..HEAD | grep -v -E '^(pkg/studio/|docs/studio/|docs/superpowers/|cmd/studio-auth-provider/|scripts/check-upstream-touchpoints.sh)')

for f in $modified; do
  found=false
  for ex in "${expected_touchpoints[@]}"; do
    if [ "$f" = "$ex" ]; then found=true; break; fi
  done
  if ! $found; then
    echo "UNEXPECTED TOUCHPOINT: $f"
    exit 1
  fi
done
echo "All upstream touchpoints accounted for."
```

- [ ] **Step 2: Document its usage in docs/studio/CHANGES.md**
- [ ] **Step 3: Commit**

```bash
git commit -m "tooling(studio): add upstream-touchpoint check script"
```

---

## Self-Review Notes

- **Spec coverage**: Each section of `mcp-obot-platform/README.md` maps to one or more tasks:
  - Studio-capable SSO provider → Task 4 (decision) + Task 19 (impl)
  - JWKS validation → Tasks 3, 5
  - Subject-based authorization → Tasks 6, 7
  - Service-identity API (ensure/rotate/disable/delete/provision/teardown/connect-URL) → Tasks 8–14
  - Custom-catalog ingester → Task 13
  - Server provisioning lifecycle → Task 11
  - Identity mapping → Task 8 + Task 19
  - Sync-with-upstream constraint → File structure principle + Tasks 1, 21

- **Placeholders**: Tasks 4, 18, 19 are explicitly investigative or branching on prior decisions — they have concrete deliverables (a decision doc, a single integration point, a chosen path) rather than placeholder code. Other tasks have concrete code, exact files, and runnable commands.

- **Type consistency**: `Principal`, `StudioConfig`, `JWTValidator`, `JWKSCache`, `IdentityStore`, `APIKeyStore`, `CatalogStore`, `RouteDeps` are introduced once and used consistently downstream.

- **Sync constraint enforced**: The three patch points are named explicitly (Tasks 15, 16, 17), and Task 21 adds tooling to detect drift on every rebase.
