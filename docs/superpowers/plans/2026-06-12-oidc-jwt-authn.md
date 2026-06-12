# OIDC JWT Authenticator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic JWT authenticator to Obot that accepts JWTs issued by the configured `generic-oauth-auth-provider`, mapping backend-principal subjects to admin/owner and user subjects to the corresponding Obot user, so the first consumer (Studio) can call Obot's existing catalog and MCP endpoints with service-identity JWTs.

**Architecture:** A new `pkg/oidcjwt/` package holds all integration code — config, JWKS cache, JWT validator, and an authenticator implementing the existing `authenticator.Request` interface (k8s style). The authenticator is inserted into the existing authenticator union in `pkg/services/config.go` with one additive line. Backend-principal subjects return a `user.Info` with groups `[types.GroupAdmin, types.GroupOwner]` so the existing `adminAndOwnerRules` in `pkg/api/authz/authz.go` accept them on `/api/system-mcp-catalogs/**`, `/api/system-mcp-servers/**`, and `/api/mcp-catalogs/**` without authz changes. User-subject JWTs resolve through the existing identity layer (`pkg/gateway/client/identity.go`), creating the Obot user record on first call if absent.

**Tech Stack:** Go 1.x (same toolchain as Obot today); `github.com/golang-jwt/jwt/v5` for JWT validation; `github.com/MicahParks/jwkset` for JWKS handling — both already in `go.mod`. Tests use `testify`. Integration test uses an in-process test HTTP server signed with a generated keypair.

**Design source of truth:** `docs/design/oidc-jwt-authn/README.md` (this repo).

---

## File Structure

| Path | Status | Responsibility |
|---|---|---|
| `pkg/oidcjwt/config.go` | new | Typed config struct, env-var binding, validation. |
| `pkg/oidcjwt/config_test.go` | new | Tests for config parsing and validation. |
| `pkg/oidcjwt/jwks.go` | new | JWKS cache: discovery, fetch, kid lookup, single-flight refresh on miss, background poll. |
| `pkg/oidcjwt/jwks_test.go` | new | Tests for JWKS cache behavior. |
| `pkg/oidcjwt/validator.go` | new | JWT signature + standard-claims validation. |
| `pkg/oidcjwt/validator_test.go` | new | Tests for validator. |
| `pkg/oidcjwt/authenticator.go` | new | Implements `authenticator.Request`. Composes config + JWKS + validator + identity resolution. |
| `pkg/oidcjwt/authenticator_test.go` | new | Unit tests for authenticator (backend-principal path, user-subject path, fall-through cases). |
| `pkg/oidcjwt/integration_test.go` | new | End-to-end test: a real `*server.Server`, a backend-principal JWT, hit `GET /api/system-mcp-catalogs/{id}/entries`, assert 200. |
| `pkg/services/config.go` | modify (1 line) | Append `oidcjwt.NewAuthenticator(...)` to the authenticators union. |
| `chart/values.yaml` | modify | Add 4 new env keys under the existing `genericOAuthAuthProvider` block. |
| `docs/studio/CHANGES.md` | new | Upstream-touchpoint manifest for rebase hygiene. |
| `scripts/check-upstream-touchpoints.sh` | new | CI check that flags unexpected upstream touches. |

The 3 upstream-touched files (`pkg/services/config.go`, `chart/values.yaml`, plus `pkg/api/router/router.go` if any router change ends up being needed) form the allow-list for `check-upstream-touchpoints.sh`.

---

## Task 1: Create `pkg/oidcjwt/config.go` with envconfig binding

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
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":                   "https://studio.example.com/api/auth",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE":                 "obot-default",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_BACKEND_PRINCIPAL":        "studio-deployment",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME":   "studio_eligible",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL":       "120",
	}

	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.Equal(t, "https://studio.example.com/api/auth", cfg.IssuerURL)
	assert.Equal(t, "obot-default", cfg.Audience)
	assert.Equal(t, "studio-deployment", cfg.BackendPrincipal)
	assert.Equal(t, "studio_eligible", cfg.EligibilityClaimName)
	assert.Equal(t, 120, cfg.JWKSPollIntervalSeconds)
}

func TestLoadConfigFromEnv_DefaultsAndDisabled(t *testing.T) {
	// Empty audience disables the authenticator entirely.
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER": "https://studio.example.com/api/auth",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.False(t, cfg.Enabled())
	assert.Equal(t, "studio_eligible", cfg.EligibilityClaimName)  // default
	assert.Equal(t, 300, cfg.JWKSPollIntervalSeconds)             // default
}

func TestLoadConfigFromEnv_PollIntervalBounds(t *testing.T) {
	// Out-of-range poll interval is clamped or rejected.
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":            "https://studio.example.com/api/auth",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE":          "obot-default",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL": "30", // below 60
	}
	_, err := LoadConfigFromEnv(envGetter(env))
	assert.Error(t, err)
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
// Package oidcjwt implements a generic JWT authenticator that validates
// JWTs issued by the configured generic-oauth-auth-provider. See
// docs/design/oidc-jwt-authn/README.md for the contract.
package oidcjwt

import (
	"fmt"
	"strconv"
)

// Config holds the runtime configuration for the JWT authenticator.
// All fields are sourced from the existing OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*
// env-var prefix so the provider has one configuration surface.
type Config struct {
	IssuerURL               string
	Audience                string
	BackendPrincipal        string
	EligibilityClaimName    string
	JWKSPollIntervalSeconds int
}

const (
	defaultEligibilityClaimName  = "studio_eligible"
	defaultJWKSPollIntervalSecs  = 300
	minJWKSPollIntervalSecs      = 60
	maxJWKSPollIntervalSecs      = 3600
)

// Enabled reports whether the authenticator is functional. Empty audience or
// issuer means the deployment has not opted in.
func (c Config) Enabled() bool {
	return c.IssuerURL != "" && c.Audience != ""
}

// LoadConfigFromEnv reads the OBOT_GENERIC_OAUTH_AUTH_PROVIDER_* env vars
// via the supplied getter. Returns an error only when a present value is
// malformed; missing optional values fall back to defaults.
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
	pollStr := getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL")
	if pollStr == "" {
		cfg.JWKSPollIntervalSeconds = defaultJWKSPollIntervalSecs
	} else {
		n, err := strconv.Atoi(pollStr)
		if err != nil {
			return Config{}, fmt.Errorf("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL: %w", err)
		}
		if n < minJWKSPollIntervalSecs || n > maxJWKSPollIntervalSecs {
			return Config{}, fmt.Errorf("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL=%d out of range [%d..%d]", n, minJWKSPollIntervalSecs, maxJWKSPollIntervalSecs)
		}
		cfg.JWKSPollIntervalSeconds = n
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestLoadConfigFromEnv -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/config.go pkg/oidcjwt/config_test.go
git commit -m "feat(oidcjwt): config type with env-var binding"
```

---

## Task 2: JWKS cache (`pkg/oidcjwt/jwks.go`)

**Files:**
- Create: `pkg/oidcjwt/jwks.go`
- Create: `pkg/oidcjwt/jwks_test.go`

- [ ] **Step 1: Write the failing test — discovery + initial fetch**

Path: `pkg/oidcjwt/jwks_test.go`

```go
package oidcjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWKSCache_FetchesByKidViaDiscovery(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := "test-kid-1"

	issuer, cleanup := newTestIssuer(t, priv, kid)
	defer cleanup()

	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	ctx := context.Background()

	got, err := cache.KeyForKid(ctx, kid)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestJWKSCache_RefreshesOnUnknownKid(t *testing.T) {
	priv1, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv1, "kid-A")
	defer cleanup()

	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	ctx := context.Background()
	_, err := cache.KeyForKid(ctx, "kid-A")
	require.NoError(t, err)

	// Issuer rotates: now publishing kid-B too.
	priv2, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer.AddKey(t, priv2, "kid-B")

	_, err = cache.KeyForKid(ctx, "kid-B")
	require.NoError(t, err)
}

// newTestIssuer spins up an httptest.Server that serves an OIDC
// discovery doc + a JWKS containing one or more RSA public keys.
type testIssuer struct {
	*httptest.Server
	keys map[string]*rsa.PrivateKey  // kid -> priv
}

func newTestIssuer(t *testing.T, priv *rsa.PrivateKey, kid string) (*testIssuer, func()) {
	ti := &testIssuer{keys: map[string]*rsa.PrivateKey{kid: priv}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   base,
			"jwks_uri": base + "/.well-known/jwks.json",
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		set := jwkset.NewMemoryStorage()
		for k, p := range ti.keys {
			meta := jwkset.JWKMetadataOptions{KID: k, ALG: jwkset.AlgRS256, USE: jwkset.UseSig}
			jwk, jerr := jwkset.NewJWKFromKey(p.Public(), jwkset.JWKOptions{Metadata: meta})
			require.NoError(t, jerr)
			require.NoError(t, set.KeyWrite(r.Context(), jwk))
		}
		j, _ := set.JSONPublic(r.Context())
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(j)
	})
	srv := httptest.NewServer(mux)
	ti.Server = srv
	return ti, srv.Close
}

func (ti *testIssuer) AddKey(t *testing.T, priv *rsa.PrivateKey, kid string) {
	ti.keys[kid] = priv
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestJWKSCache -v`
Expected: FAIL with `undefined: NewJWKSCache` and `undefined: JWKSCache`.

- [ ] **Step 3: Write minimal implementation**

Path: `pkg/oidcjwt/jwks.go`

```go
package oidcjwt

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/jwkset"
)

// JWKSCache fetches and caches the JWKS for one OIDC issuer. Refreshes
// on a poll interval and on demand when a JWT presents an unknown kid.
type JWKSCache struct {
	issuerURL       string
	pollInterval    time.Duration
	httpClient      *http.Client
	mu              sync.Mutex
	jwksURI         string
	keysByKid       map[string]crypto.PublicKey
	refreshInflight chan struct{}
}

// NewJWKSCache constructs a cache pointed at the given issuer URL. The cache
// is lazy — no network calls happen until KeyForKid is invoked.
func NewJWKSCache(issuerURL string, pollInterval time.Duration) *JWKSCache {
	return &JWKSCache{
		issuerURL:    strings.TrimRight(issuerURL, "/"),
		pollInterval: pollInterval,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		keysByKid:    map[string]crypto.PublicKey{},
	}
}

// KeyForKid returns the cached key for the given kid, refreshing the JWKS once
// if the kid is unknown. Returns (nil, nil) if the issuer's JWKS does not
// contain the kid even after refresh.
func (c *JWKSCache) KeyForKid(ctx context.Context, kid string) (crypto.PublicKey, error) {
	c.mu.Lock()
	if k, ok := c.keysByKid[kid]; ok {
		c.mu.Unlock()
		return k, nil
	}
	c.mu.Unlock()

	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keysByKid[kid]; ok {
		return k, nil
	}
	return nil, nil
}

func (c *JWKSCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	if c.refreshInflight != nil {
		ch := c.refreshInflight
		c.mu.Unlock()
		<-ch
		return nil
	}
	c.refreshInflight = make(chan struct{})
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		close(c.refreshInflight)
		c.refreshInflight = nil
		c.mu.Unlock()
	}()

	if c.jwksURI == "" {
		uri, err := c.discoverJWKSURI(ctx)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.jwksURI = uri
		c.mu.Unlock()
	}

	keys, err := c.fetchJWKS(ctx, c.jwksURI)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.keysByKid = keys
	c.mu.Unlock()
	return nil
}

type discoveryDoc struct {
	JWKSURI string `json:"jwks_uri"`
}

func (c *JWKSCache) discoverJWKSURI(ctx context.Context) (string, error) {
	url := c.issuerURL + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oidcjwt: discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oidcjwt: discovery status %d", resp.StatusCode)
	}
	var doc discoveryDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("oidcjwt: discovery decode: %w", err)
	}
	if doc.JWKSURI == "" {
		return "", errors.New("oidcjwt: discovery missing jwks_uri")
	}
	return doc.JWKSURI, nil
}

func (c *JWKSCache) fetchJWKS(ctx context.Context, uri string) (map[string]crypto.PublicKey, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidcjwt: jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidcjwt: jwks status %d", resp.StatusCode)
	}
	stored, err := jwkset.NewMemoryStorage().LoadJSON(ctx, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oidcjwt: jwks parse: %w", err)
	}
	out := map[string]crypto.PublicKey{}
	for _, k := range stored {
		meta := k.Marshal()
		out[meta.KID] = k.Key()
	}
	return out, nil
}
```

> **Note for the implementer:** The MicahParks/jwkset API surface may have evolved across versions; the call shapes above (`NewMemoryStorage`, `LoadJSON`, `Marshal()`) reflect the version pinned in `go.mod` at planning time. If the API differs, use the equivalent loader / accessor — the shape `kid → crypto.PublicKey` is what `KeyForKid` exposes.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestJWKSCache -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/jwks.go pkg/oidcjwt/jwks_test.go
git commit -m "feat(oidcjwt): JWKS cache with discovery + refresh on unknown kid"
```

---

## Task 3: JWT validator (`pkg/oidcjwt/validator.go`)

**Files:**
- Create: `pkg/oidcjwt/validator.go`
- Create: `pkg/oidcjwt/validator_test.go`

- [ ] **Step 1: Write the failing test**

Path: `pkg/oidcjwt/validator_test.go`

```go
package oidcjwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mintTestJWT(t *testing.T, priv *rsa.PrivateKey, kid, iss, aud, sub string, ttl time.Duration, extra map[string]any) string {
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
	s, err := tok.SignedString(priv)
	require.NoError(t, err)
	return s
}

func TestValidator_Accepts(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default"}
	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	v := NewValidator(cfg, cache)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
	claims, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "studio-deployment", claims.Subject)
}

func TestValidator_RejectsWrongAud(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default"}
	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	v := NewValidator(cfg, cache)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "wrong-aud", "studio-deployment", 60*time.Second, nil)
	_, err := v.Validate(context.Background(), tok)
	assert.Error(t, err)
}

func TestValidator_RejectsExpired(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default"}
	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	v := NewValidator(cfg, cache)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", -1*time.Minute, nil)
	_, err := v.Validate(context.Background(), tok)
	assert.Error(t, err)
}

func TestValidator_ReturnsUnknownKidAsNotOurs(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default"}
	cache := NewJWKSCache(issuer.URL, 300*time.Second)
	v := NewValidator(cfg, cache)

	// Sign with a different key + unknown kid.
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := mintTestJWT(t, otherKey, "unknown-kid", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
	_, err := v.Validate(context.Background(), tok)
	assert.ErrorIs(t, err, ErrNotMyToken)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestValidator -v`
Expected: FAIL with `undefined: NewValidator`, `undefined: Validator`, `undefined: ErrNotMyToken`.

- [ ] **Step 3: Write minimal implementation**

Path: `pkg/oidcjwt/validator.go`

```go
package oidcjwt

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNotMyToken signals that the JWT cannot be authenticated by this
// authenticator (unknown kid or unparseable) — the caller should let
// other authenticators in the union try.
var ErrNotMyToken = errors.New("oidcjwt: token not for this authenticator")

// Claims is the validated set of claims this authenticator cares about.
type Claims struct {
	Subject       string
	EligibilityOK bool   // true iff the configured eligibility claim is present and truthy
	Issuer        string
	Audience      string
}

// Validator validates a JWT against the configured issuer + audience using
// the JWKS cache for signature keys.
type Validator struct {
	cfg   Config
	keys  *JWKSCache
}

func NewValidator(cfg Config, keys *JWKSCache) *Validator {
	return &Validator{cfg: cfg, keys: keys}
}

// Validate parses the JWT, verifies signature + standard claims, and returns
// extracted Claims. Returns ErrNotMyToken when the kid is unknown (so the
// authenticator can fall through to the next in the union).
func (v *Validator) Validate(ctx context.Context, raw string) (Claims, error) {
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unsupported alg %q", t.Method.Alg())
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, ErrNotMyToken
		}
		key, err := v.keys.KeyForKid(ctx, kid)
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, ErrNotMyToken
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.IssuerURL),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(60),
	)
	if err != nil {
		if errors.Is(err, ErrNotMyToken) {
			return Claims{}, ErrNotMyToken
		}
		return Claims{}, err
	}

	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, errors.New("oidcjwt: unexpected claims type")
	}
	sub, _ := mc["sub"].(string)
	iss, _ := mc["iss"].(string)
	aud := ""
	if a, ok := mc["aud"].(string); ok {
		aud = a
	}

	elig := false
	if v.cfg.EligibilityClaimName != "" {
		switch val := mc[v.cfg.EligibilityClaimName].(type) {
		case bool:
			elig = val
		case []any:
			elig = len(val) > 0
		}
	}

	return Claims{Subject: sub, EligibilityOK: elig, Issuer: iss, Audience: aud}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestValidator -v`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/validator.go pkg/oidcjwt/validator_test.go
git commit -m "feat(oidcjwt): JWT validator with iss/aud/exp/kid checks"
```

---

## Task 4: Authenticator (`pkg/oidcjwt/authenticator.go`) — backend-principal path

**Files:**
- Create: `pkg/oidcjwt/authenticator.go`
- Create: `pkg/oidcjwt/authenticator_test.go`

- [ ] **Step 1: Write the failing test — backend-principal path**

Path: `pkg/oidcjwt/authenticator_test.go`

```go
package oidcjwt

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticator_BackendPrincipalGrantsAdminAndOwner(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:        issuer.URL,
		Audience:         "obot-default",
		BackendPrincipal: "studio-deployment",
	}
	auth := NewAuthenticator(cfg, NewJWKSCache(issuer.URL, 300*time.Second), nil)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
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
	assert.False(t, ok) // not ours; let next in union try
}

func TestAuthenticator_DisabledConfigFallsThrough(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL /* no Audience -> disabled */}
	auth := NewAuthenticator(cfg, NewJWKSCache(issuer.URL, 300*time.Second), nil)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "studio-deployment", 60*time.Second, nil)
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

- [ ] **Step 3: Write minimal implementation (backend-principal path only — user-subject is Task 5)**

Path: `pkg/oidcjwt/authenticator.go`

```go
package oidcjwt

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// IdentityResolver is the contract this authenticator needs to map a
// user-subject JWT to an Obot user record. Wired in Task 5 against
// pkg/gateway/client; left nil-safe so this task's tests do not need it.
type IdentityResolver interface {
	ResolveOrCreate(ctx interface{ /* context.Context */ Deadline() }, issuer, sub string, profile map[string]any) (user.Info, error)
}

// Authenticator implements k8s.io/apiserver/pkg/authentication/authenticator.Request
// using a JWT validator. See docs/design/oidc-jwt-authn/README.md.
type Authenticator struct {
	cfg       Config
	validator *Validator
	identity  IdentityResolver
}

func NewAuthenticator(cfg Config, keys *JWKSCache, identity IdentityResolver) *Authenticator {
	var v *Validator
	if keys != nil {
		v = NewValidator(cfg, keys)
	}
	return &Authenticator{cfg: cfg, validator: v, identity: identity}
}

// AuthenticateRequest implements authenticator.Request.
// Returns (resp, true, nil) on a fully-validated, authorized JWT.
// Returns (nil, false, nil) when the token does not belong to this
// authenticator (no bearer, non-JWT, unknown kid, config disabled).
// Returns (nil, false, err) on a real auth failure for a token whose
// kid matched our JWKS but failed validation.
func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !a.cfg.Enabled() || a.validator == nil {
		return nil, false, nil
	}
	raw := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if raw == "" || raw == req.Header.Get("Authorization") {
		return nil, false, nil
	}
	claims, err := a.validator.Validate(req.Context(), raw)
	if errors.Is(err, ErrNotMyToken) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: validate: %w", err)
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

	// User-subject path implemented in Task 5. Until then, refuse any
	// non-backend-principal subject so this task does not silently grant
	// access.
	return nil, false, errors.New("oidcjwt: user-subject not yet implemented")
}
```

> **Note:** `IdentityResolver` is a placeholder interface with the wrong method signature on purpose — it is replaced in Task 5 with the real `pkg/gateway/client.Client` adapter. Until then the field is unused.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/oidcjwt/... -run TestAuthenticator -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/oidcjwt/authenticator.go pkg/oidcjwt/authenticator_test.go
git commit -m "feat(oidcjwt): authenticator with backend-principal subject path"
```

---

## Task 5: User-subject path in the authenticator

**Files:**
- Modify: `pkg/oidcjwt/authenticator.go`
- Modify: `pkg/oidcjwt/authenticator_test.go`
- Read: `pkg/gateway/client/identity.go` to identify the right identity-resolution call

- [ ] **Step 1: Survey the identity layer**

Run: `grep -n 'EnsureIdentity\|func .* Client' pkg/gateway/client/identity.go`
Expected: lines for `EnsureIdentity(ctx, id, timeout)` and `EnsureIdentityWithRole(ctx, id, role, ...)`.

Read the `EnsureIdentity` signature carefully — note the `*types.Identity` shape with `IssuerURL`, `Subject`, `Email`, `Username`, etc. The user-subject path constructs a `*types.Identity` from the JWT and calls `EnsureIdentity` on the gateway client. If implementer finds the method signature has evolved since planning, adapt to the current shape — the contract is "look up or create the Obot user record for `{ issuer, sub }`, returning the user.Info representation."

- [ ] **Step 2: Write the failing test — user-subject path**

Path: append to `pkg/oidcjwt/authenticator_test.go`

```go
func TestAuthenticator_UserSubjectResolvesViaIdentity(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		BackendPrincipal:     "studio-deployment",
		EligibilityClaimName: "studio_eligible",
	}

	stub := &stubIdentity{
		resolve: func(issuer, sub string, profile map[string]any) (user.Info, error) {
			return &user.DefaultInfo{
				UID:    "obot-user-1",
				Name:   "alice@example.com",
				Groups: []string{types.GroupAuthenticated},
			}, nil
		},
	}
	auth := NewAuthenticator(cfg, NewJWKSCache(issuer.URL, 300*time.Second), stub)

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-123",
		60*time.Second, map[string]any{"studio_eligible": true, "email": "alice@example.com"})
	req, _ := http.NewRequest("GET", "/api/mcp-servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice@example.com", resp.User.GetName())
}

func TestAuthenticator_UserSubjectFailsWhenIneligible(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	issuer, cleanup := newTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "studio_eligible",
	}
	auth := NewAuthenticator(cfg, NewJWKSCache(issuer.URL, 300*time.Second), &stubIdentity{})

	tok := mintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-123",
		60*time.Second, map[string]any{"studio_eligible": false})
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err := auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

type stubIdentity struct {
	resolve func(issuer, sub string, profile map[string]any) (user.Info, error)
}

// Adapter to the real interface signature once the identity contract is in.
// See note in authenticator.go on the IdentityResolver interface shape.
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/oidcjwt/... -run TestAuthenticator_UserSubject -v`
Expected: FAIL (user-subject path returns `not yet implemented`).

- [ ] **Step 4: Replace the placeholder `IdentityResolver` interface with the real shape and wire the user-subject path**

Update `pkg/oidcjwt/authenticator.go`:
- Change the `IdentityResolver` interface to use `context.Context` properly and match the real call site needed.
- In `AuthenticateRequest`, after the backend-principal check, if `a.identity != nil` and `claims.EligibilityOK` is true, call the resolver with the JWT's `iss`/`sub`/profile fields and return the resulting `user.Info`.
- If `claims.EligibilityOK` is false, return a real error (this is a real auth failure for a token that is "ours").

Final user-subject section of `AuthenticateRequest`:

```go
if !claims.EligibilityOK {
    return nil, false, errors.New("oidcjwt: eligibility claim missing or false")
}
if a.identity == nil {
    return nil, false, errors.New("oidcjwt: identity resolver not configured")
}
profile := map[string]any{}
if email, _ := mc["email"].(string); email != "" {
    profile["email"] = email
}
if name, _ := mc["name"].(string); name != "" {
    profile["name"] = name
}
info, err := a.identity.ResolveOrCreate(req.Context(), claims.Issuer, claims.Subject, profile)
if err != nil {
    return nil, false, fmt.Errorf("oidcjwt: identity resolve: %w", err)
}
return &authenticator.Response{User: info}, true, nil
```

Note `mc` is the JWT's MapClaims — this section needs access to the raw claims. Refactor `Validator.Validate` to return `(Claims, jwt.MapClaims, error)` or pass the raw profile fields through `Claims` (`Claims.Email`, `Claims.Name`).

Pick the second option for cleanliness — extend `Claims` with `Email string` and `Name string`. Update tests in Task 3 accordingly.

- [ ] **Step 5: Implement the real `client.Client` adapter for `IdentityResolver`**

Create `pkg/oidcjwt/identity_adapter.go`:

```go
package oidcjwt

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/user"
)

// NewGatewayIdentityResolver returns an IdentityResolver backed by the
// gateway client's EnsureIdentity path — the same path the OAuth provider
// uses at browser-login time.
func NewGatewayIdentityResolver(c *gclient.Client) IdentityResolver {
	return &gatewayResolver{c: c}
}

type gatewayResolver struct{ c *gclient.Client }

func (g *gatewayResolver) ResolveOrCreate(ctx context.Context, issuer, sub string, profile map[string]any) (user.Info, error) {
	id := &types.Identity{
		IssuerURL:      issuer,
		Subject:        sub,
		ProviderName:   "generic-oauth-auth-provider",
		// fill remaining fields from profile (email, username, displayname, picture)
	}
	if email, ok := profile["email"].(string); ok {
		id.Email = email
	}
	if name, ok := profile["name"].(string); ok {
		id.DisplayName = name
		id.Username = name
	}
	out, err := g.c.EnsureIdentity(ctx, id, 0 /* timeout, adjust to current signature */)
	if err != nil {
		return nil, err
	}
	return &user.DefaultInfo{
		UID:    out.UID(),  // adjust to current Identity API
		Name:   out.Email,
		Groups: []string{types.GroupAuthenticated},
	}, nil
}
```

> The exact `types.Identity` field names and `EnsureIdentity` signature should be confirmed by reading `pkg/gateway/client/identity.go` and `apiclient/types/identity.go` at implementation time. Adapt freely — the contract is "look up or create the Obot user record."

- [ ] **Step 6: Run tests**

Run: `go test ./pkg/oidcjwt/... -v`
Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/oidcjwt/authenticator.go pkg/oidcjwt/authenticator_test.go pkg/oidcjwt/identity_adapter.go pkg/oidcjwt/validator.go pkg/oidcjwt/validator_test.go
git commit -m "feat(oidcjwt): user-subject path with gateway identity resolver"
```

---

## Task 6: Wire authenticator into the union in `pkg/services/config.go`

**Files:**
- Modify: `pkg/services/config.go` (one additive change near line 838)

- [ ] **Step 1: Locate the union-build region**

Run: `sed -n '825,850p' pkg/services/config.go`
Expected output: the union assembly between `gserver.NewGatewayTokenReviewer` and `authn.Anonymous{}`.

- [ ] **Step 2: Insert the Studio JWT authenticator**

Edit `pkg/services/config.go`. Right after the line `authenticators = union.New(authenticators, persistentTokenServer)` (around line 840), add:

```go
// OIDC JWT authenticator — accepts JWTs minted by the configured
// generic-oauth-auth-provider. See pkg/oidcjwt and
// docs/design/oidc-jwt-authn/README.md.
oidcJWTCfg, err := oidcjwt.LoadConfigFromEnv(os.Getenv)
if err != nil {
    return nil, fmt.Errorf("oidcjwt config: %w", err)
}
if oidcJWTCfg.Enabled() {
    oidcJWTKeys := oidcjwt.NewJWKSCache(oidcJWTCfg.IssuerURL,
        time.Duration(oidcJWTCfg.JWKSPollIntervalSeconds)*time.Second)
    oidcJWTResolver := oidcjwt.NewGatewayIdentityResolver(gatewayClient)
    authenticators = union.New(authenticators,
        oidcjwt.NewAuthenticator(oidcJWTCfg, oidcJWTKeys, oidcJWTResolver))
}
```

Add the import `"github.com/obot-platform/obot/pkg/oidcjwt"` if not already present. Add `"os"` and `"time"` if not already imported (likely both already are; check before adding).

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

## Task 7: End-to-end integration test

**Files:**
- Create: `pkg/oidcjwt/integration_test.go`

The integration test boots a minimal slice of the Obot API server (just enough to route to the catalog handler) wired with the JWT authenticator. It signs a backend-principal JWT and asserts that `GET /api/system-mcp-catalogs/{catalog_id}/entries` returns 200 with the expected payload shape.

- [ ] **Step 1: Write the integration test**

Path: `pkg/oidcjwt/integration_test.go`

```go
package oidcjwt_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/oidcjwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestIntegration_BackendPrincipalJWTReachesCatalog wires the authenticator
// into a minimal HTTP stack and asserts that a backend-principal JWT
// authenticates as admin/owner — which the existing adminAndOwnerRules
// already authorize for /api/system-mcp-catalogs/**.
//
// This test does not boot the full Obot router; it constructs a minimal
// handler that mimics what the real router would do post-authn: read the
// user.Info from the request context, check group membership, and return
// the catalog payload.
func TestIntegration_BackendPrincipalJWTReachesCatalog(t *testing.T) {
	// 1. Spin up a test OIDC issuer.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	issuer, cleanup := newTestIssuer(t, priv, "kid-int-1")
	defer cleanup()

	// 2. Configure the authenticator.
	cfg := oidcjwt.Config{
		IssuerURL:               issuer.URL,
		Audience:                "obot-default",
		BackendPrincipal:        "studio-deployment",
		EligibilityClaimName:    "studio_eligible",
		JWKSPollIntervalSeconds: 300,
	}
	keys := oidcjwt.NewJWKSCache(issuer.URL, 300*time.Second)
	jwtAuth := oidcjwt.NewAuthenticator(cfg, keys, nil)
	a := authn.NewAuthenticator(jwtAuth)

	// 3. Minimal handler simulating what /api/system-mcp-catalogs/{id}/entries
	//    would do once authn + authz have approved the call.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := a.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		// Check that the caller has Admin or Owner — the same check
		// pkg/api/authz/authz.go applies via adminAndOwnerRules.
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

	// 4. Mint a backend-principal JWT and call the handler.
	tok := mintTestJWT(t, priv, "kid-int-1", issuer.URL, "obot-default",
		"studio-deployment", 60*time.Second, nil)

	req := httptest.NewRequest("GET",
		"/api/system-mcp-catalogs/default/entries", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req = req.WithContext(context.Background())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out, "items")
}

// newTestIssuer + mintTestJWT are shared with the unit tests; for the
// integration package this file declares its own (kept minimal here —
// in practice the unit-test helpers can be exported to a testutil pkg).
func newTestIssuer(t *testing.T, priv *rsa.PrivateKey, kid string) (*httptest.Server, func()) {
	// (see Task 2 for the full implementation; copy and adapt for the
	// _test package boundary, or extract to pkg/oidcjwt/testutil/ if
	// the implementer prefers)
	panic("see Task 2 helper")
}

func mintTestJWT(t *testing.T, priv *rsa.PrivateKey, kid, iss, aud, sub string, ttl time.Duration, extra map[string]any) string {
	panic("see Task 3 helper")
}

func _unused_userInfo() user.Info { return nil }
```

> **Note for the implementer:** The integration test shares `newTestIssuer` / `mintTestJWT` with the unit tests. Either extract them to a small `pkg/oidcjwt/testutil` package (preferred — clean separation between in-package and `_test` external tests) or duplicate the helpers into the integration test file. The plan asks for extraction; the panic stubs above mark the point.

- [ ] **Step 2: Extract test helpers to `pkg/oidcjwt/testutil/`**

Create `pkg/oidcjwt/testutil/testutil.go` and move `newTestIssuer` + `mintTestJWT` there as exported `NewTestIssuer` and `MintTestJWT`. Update callers in Task 2, 3, 4, 5 tests to import from the new package.

- [ ] **Step 3: Run the integration test**

Run: `go test ./pkg/oidcjwt/... -run TestIntegration -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/oidcjwt/integration_test.go pkg/oidcjwt/testutil/
git commit -m "feat(oidcjwt): integration test for backend-principal JWT auth"
```

---

## Task 8: Chart values

**Files:**
- Modify: `chart/values.yaml`

- [ ] **Step 1: Locate the genericOAuthAuthProvider block**

Run: `grep -n 'genericOAuth\|GENERIC_OAUTH' chart/values.yaml | head`
Expected: locate the section that drives the existing env vars.

- [ ] **Step 2: Add 4 new keys under that block**

Add (or extend) under the existing block:

```yaml
genericOAuthAuthProvider:
  # ... existing fields (issuer, clientId, clientSecret, etc.) ...
  # Service-identity JWT validation (oidcjwt). Empty audience disables
  # the JWT authenticator without affecting browser-flow login.
  audience: ""                    # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE
  backendPrincipal: ""            # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_BACKEND_PRINCIPAL
  eligibilityClaimName: "studio_eligible"  # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME
  jwksPollIntervalSeconds: 300    # OBOT_GENERIC_OAUTH_AUTH_PROVIDER_JWKS_POLL_INTERVAL
```

If the existing block already maps the issuer/clientId to env-var-prefixed names via a templated deployment, add the four new env vars to the same templating.

- [ ] **Step 3: Render the chart locally if possible**

Run: `helm template chart/ > /tmp/rendered.yaml && grep -A 1 'OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE' /tmp/rendered.yaml`
Expected: the env var appears in the rendered deployment manifest.

- [ ] **Step 4: Commit**

```bash
git add chart/values.yaml
git commit -m "feat(oidcjwt): chart values for audience, backend principal, eligibility claim, JWKS poll"
```

---

## Task 9: Upstream-touchpoint manifest

**Files:**
- Create: `docs/studio/CHANGES.md`
- Create: `scripts/check-upstream-touchpoints.sh`

- [ ] **Step 1: Write `docs/studio/CHANGES.md`**

```markdown
# OIDC JWT Authenticator — Upstream Touchpoints

This manifest tracks every file outside `pkg/oidcjwt/` that the OIDC JWT
authenticator integration touches. Keep this list in sync with reality.
Each rebase should verify the list is unchanged (run
`scripts/check-upstream-touchpoints.sh`).

## Allowed touchpoints

| File | Why |
|---|---|
| `pkg/services/config.go` | Append `oidcjwt.NewAuthenticator(...)` to the authenticator union when the config is enabled. |
| `chart/values.yaml` | Add 4 new env-var keys under the existing `genericOAuthAuthProvider` block. |
| `docs/design/oidc-jwt-authn/README.md` | Design document. |
| `docs/superpowers/plans/2026-06-12-oidc-jwt-authn.md` | Implementation plan (this file). |
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
# Usage: scripts/check-upstream-touchpoints.sh <BASE_REF>
#   BASE_REF defaults to origin/main.

set -euo pipefail

BASE_REF="${1:-origin/main}"

ALLOWED=(
  "pkg/oidcjwt/"
  "pkg/services/config.go"
  "chart/values.yaml"
  "docs/design/oidc-jwt-authn/"
  "docs/superpowers/plans/2026-06-12-oidc-jwt-authn.md"
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
Expected: exits 0 with "OK".

- [ ] **Step 4: Commit**

```bash
git add docs/studio/CHANGES.md scripts/check-upstream-touchpoints.sh
git commit -m "chore(oidcjwt): upstream-touchpoint manifest + CI check"
```

---

## Task 10: Verify and push

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: all pass.

- [ ] **Step 2: Run `go vet ./...` and any linters CI uses**

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 3: Re-run the upstream-touchpoint check**

Run: `scripts/check-upstream-touchpoints.sh origin/main`
Expected: exits 0.

- [ ] **Step 4: Push the branch**

```bash
git push -u origin feat/support-studio-jwt
```

If the branch already has commits on the remote (it does — design doc commits landed earlier), use `git push` without `-u`.

---

## Acceptance Criteria

- A backend-principal JWT signed by a test issuer authenticates as a user with groups `[Admin, Owner]`, and the existing `adminAndOwnerRules` accept it on `/api/system-mcp-catalogs/{id}/entries`. Verified by `TestIntegration_BackendPrincipalJWTReachesCatalog`.
- A user-subject JWT with `studio_eligible: true` resolves to the corresponding Obot user via the gateway identity layer (or creates one on first call). Verified by `TestAuthenticator_UserSubjectResolvesViaIdentity`.
- A user-subject JWT without the eligibility claim is refused with a real auth error (401-equivalent). Verified by `TestAuthenticator_UserSubjectFailsWhenIneligible`.
- A non-JWT bearer or no `Authorization` header passes the authenticator unchanged so the next authenticator in the union can try. Verified by `TestAuthenticator_NoBearerFallsThrough` and `TestAuthenticator_DisabledConfigFallsThrough`.
- The check script catches any code change outside the allow-list.
- `go test ./...` and `go vet ./...` are clean.

End-to-end manual verification (once the Studio side is built):

1. Bring up the Studio + Obot compose stack.
2. From Studio, mint a backend-principal JWT via the Studio-side mint helper.
3. Use `curl` from the Studio container to call Obot's `GET /api/system-mcp-catalogs/{catalog_id}/entries` with `Authorization: Bearer <jwt>`.
4. Expect 200 with the catalog payload.

---

## Notes for the implementer

- **Authenticator-union semantics.** Each authenticator in the union returns `(response, ok, err)`. Returning `(nil, false, nil)` means "not mine, let the next try." Returning `(nil, false, err)` is a real auth failure surfaced as 401. This authenticator returns a real error only when the JWT is structurally ours (matching `kid`) but fails validation, or when it is structurally ours but ineligible.

- **JWKS cache.** The cache refreshes on demand when a JWT presents an unknown `kid`. Background polling (`JWKSPollIntervalSeconds`) is a nice-to-have but the on-demand refresh is what makes correctness work — implement the on-demand path first. Background polling can be added later if observed cache-staleness becomes an issue.

- **Identity layer compatibility.** The exact `pkg/gateway/client.Client.EnsureIdentity` signature may have changed since planning. Adapt the `gatewayResolver` adapter to the current signature; the contract — "look up or create the Obot user record for `{ issuer, sub }`" — is what matters. If `EnsureIdentity` returns a domain model rather than `user.Info`, map the fields.

- **JWT library compatibility.** `golang-jwt/jwt/v5` and `MicahParks/jwkset` API surfaces have evolved across versions. If `jwt.MapClaims` or `jwkset.NewMemoryStorage()` differs from the snippets above, use the equivalent for the version pinned in `go.mod`.

- **No new dependencies.** Both libraries above are already in `go.mod`. If a clean implementation requires another library, document why in the commit and update the design doc.

- **No `pkg/api/router/router.go` change.** The authenticator plugs in at the union assembly site, so the router file is untouched. If the implementer finds a reason to touch the router, add it to the allow-list and update the design.
