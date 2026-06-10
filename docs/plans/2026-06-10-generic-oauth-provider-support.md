# Generic OAuth Provider Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one first-class generic OAuth / OIDC auth provider that admins configure with issuer, client ID, client secret, scope, email domains, provider name, and issuer-scoped trusted account linking.

**Architecture:** Obot will expose one registry-backed `generic-oauth-auth-provider` type through the existing auth-provider API/UI. The provider daemon performs OIDC protocol work and returns an issuer-bound provider user ID so existing identity storage can distinguish two issuers that produce the same `sub`. Trusted email linking becomes generic-provider configuration, scoped to the validated issuer rather than a global provider-name allowlist.

**Tech Stack:** Go 1.26, GORM, Obot gateway/auth-provider dispatcher, SvelteKit 5, TypeScript, provider registry YAML, external `ghcr.io/obot-platform/providers` image.

---

## Source Design

- Design spec: `docs/design/generic-oauth-provider/README.md`
- Plan location: `docs/plans/2026-06-10-generic-oauth-provider-support.md`

## File Structure

Main Obot repository:

- Modify `apiclient/types/authprovider.go` to add generic provider constants if shared API code needs stable names.
- Modify `pkg/auth/auth.go` and `pkg/proxy/proxy.go` to pass issuer and email verification metadata from the provider daemon into gateway identity creation.
- Modify `pkg/gateway/types/identity.go` only if the implementation chooses a persisted issuer field. This plan recommends issuer-bound provider UID instead, avoiding a schema migration.
- Modify `pkg/gateway/client/auth.go` to copy issuer/email verification metadata into `types.Identity`.
- Modify `pkg/gateway/client/identity.go` to replace static provider-name trust with a helper that handles GitHub/Google plus generic provider trust config.
- Modify `pkg/gateway/client/user.go` to map generic profile fields for display name and icon.
- Modify `pkg/api/handlers/authprovider.go` to validate generic provider configuration and handle issuer-change trust reconfirmation.
- Modify `pkg/api/handlers/providers/authproviders.go` to treat `Trust Email Linking` as required/defaulted generic provider config.
- Modify `pkg/controller/handlers/provider/provider.go` to fix auth-provider status credential context if needed and ensure generic required fields are reflected.
- Modify `ui/user/src/routes/admin/auth-providers/+page.svelte` to sort and configure the generic provider.
- Modify `ui/user/src/lib/components/admin/ProviderConfigure.svelte` to render the trust toggle and issuer-change warning.
- Modify `ui/user/src/lib/services/admin/types.ts` if the provider parameter model needs a boolean/toggle hint beyond name-based special casing.
- Modify `docs/docs/configuration/auth-providers.md` for operator documentation.
- Add Playwright E2E coverage under `ui/user/e2e` or a repo-level `e2e` directory, following whichever layout is introduced for the first Playwright suite.
- Add E2E fixtures under `ui/user/e2e/fixtures` or `e2e/fixtures`. All provider/client/user settings must come from fixture files, not inline constants, except the single CI gate `OBOT_E2E_KEYCLOAK=false`.
- Add or update tests under `pkg/gateway/client`, `pkg/proxy`, `pkg/api/handlers`, `pkg/controller/handlers/provider`, and `ui/user`.

Provider image/repo dependency:

- Verify whether `ghcr.io/obot-platform/providers` already contains a generic OIDC provider command.
- If it does not, add that command in the provider image source repo before enabling the Obot registry entry.
- The provider command must implement the daemon contract from the design spec: discovery, state, nonce, PKCE, ID-token validation, JWKS rotation, userinfo validation, issuer-bound UID, and `/obot-get-user-info`.

## Recommended Implementation Choice

Use issuer-bound provider UID for v1:

```text
provider user ID returned by generic daemon = "iss:" + canonicalIssuer + "\x00sub:" + sub
```

This avoids a gateway identity schema migration while preserving the OIDC guarantee that `sub` is unique only inside an issuer. It also makes `hashed_provider_user_id` safe for issuer switches. Keep the raw OIDC `sub` inside the provider daemon/profile response only if future group lookup needs it.

---

### Task 1: Verify or Add the Generic OIDC Provider Daemon

**Files:**
- External dependency: provider image source for `ghcr.io/obot-platform/providers`
- Obot reference: `Dockerfile`
- Obot reference: `pkg/gateway/server/dispatcher/dispatcher.go`
- Obot reference: `pkg/proxy/proxy.go`

- [ ] **Step 1: Verify whether the provider image has a generic OIDC command**

Run from the Obot repo:

```bash
docker run --rm ghcr.io/obot-platform/providers:latest sh -lc 'find /obot-providers -maxdepth 4 -type f | sort | grep -Ei "generic|oidc|oauth|auth-provider"'
```

Expected if it exists: output includes a command path and an `auth-providers/*.yaml` entry for generic OIDC.

Expected if it does not exist: no generic OIDC provider command appears. Continue with Step 2.

- [ ] **Step 2: If missing, add a provider daemon in the provider image source repo**

Create a generic OIDC auth-provider command in the provider repo using its existing provider-daemon conventions. The command must accept these env vars:

```text
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE
OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING
OBOT_AUTH_PROVIDER_EMAIL_DOMAINS
OBOT_AUTH_PROVIDER_COOKIE_SECRET
OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
```

The daemon's `serializableState` JSON returned from `/obot-get-state` must include:

```json
{
  "accessToken": "provider-access-token",
  "preferredUsername": "alice@example.com",
  "user": "iss:https://issuer.example.com/\u0000sub:provider-sub",
  "email": "alice@example.com",
  "issuer": "https://issuer.example.com/",
  "emailVerified": true,
  "setCookies": []
}
```

- [ ] **Step 3: Add provider-daemon tests in the provider repo**

Use a fake OIDC issuer and test:

```text
- discovery issuer mismatch fails
- missing authorization_endpoint fails
- auth URL contains state, nonce, PKCE challenge, default scope
- custom scope overrides default
- ID token issuer mismatch fails
- ID token audience mismatch fails
- expired ID token fails
- JWKS key rotation succeeds after refresh
- userinfo sub mismatch fails
- email_verified=false is returned distinctly from absent email_verified
- returned user ID is issuer-bound
```

- [ ] **Step 4: Build and publish or locally override the provider image**

For local Obot verification, build the provider image and pass it to the Obot Docker build:

```bash
docker build -t obot-providers:generic-oidc /path/to/providers-repo
docker build --build-arg PROVIDERS_IMAGE=obot-providers:generic-oidc -t obot:generic-oidc .
```

Expected: the final Obot image contains the generic OIDC provider command and registry YAML under `/obot-providers`.

---

### Task 2: Add Generic Provider Registry Metadata

**Files:**
- External provider repo: `auth-providers/generic-oauth-auth-provider.yaml`
- Test: provider repo registry/manifest tests, or Obot-side `pkg/controller/handlers/provider/provider_test.go` with a fixture manifest

- [ ] **Step 1: Add the registry manifest**

Add this manifest in the provider repo, using the actual command path from Task 1:

```yaml
name: Custom OAuth / OIDC
command: bin/generic-oauth-auth-provider
icon: ""
description: Configure a custom OAuth 2.0 / OpenID Connect identity provider.
requiredConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME
    friendlyName: Provider Name
    description: Display name shown on the login button.
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER
    friendlyName: Issuer URL
    description: OIDC issuer URL. Must expose /.well-known/openid-configuration.
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID
    friendlyName: Client ID
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET
    friendlyName: Client Secret
    sensitive: true
  - name: OBOT_AUTH_PROVIDER_EMAIL_DOMAINS
    friendlyName: Email Domains
    description: Comma-separated domains, or * to allow all issuer-accepted domains.
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING
    friendlyName: Trust this issuer for account linking
    description: Allows this issuer to link logins to existing Obot users by email.
optionalConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE
    friendlyName: Scope
    description: Space-delimited OIDC scopes. Defaults to openid email profile.
```

- [ ] **Step 2: Add an Obot registry-loader test fixture**

Modify `pkg/controller/handlers/provider/provider_test.go` to add a generic manifest fixture beside the existing GitHub fixture:

```go
if err := os.WriteFile(filepath.Join(authProvidersDir, "generic-oauth-auth-provider.yaml"), []byte(`name: Custom OAuth / OIDC
command: bin/generic-oauth-auth-provider
requiredConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET
  - name: OBOT_AUTH_PROVIDER_EMAIL_DOMAINS
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING
optionalConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE
`), 0o644); err != nil {
	t.Fatal(err)
}
```

Update the expected object count from `2` to `3`, and assert:

```go
if provider.Name == "generic-oauth-auth-provider" {
	foundGenericAuth = true
	if provider.Spec.Name != "Custom OAuth / OIDC" {
		t.Fatalf("expected generic auth provider display name Custom OAuth / OIDC, got %q", provider.Spec.Name)
	}
	if provider.Spec.Command != filepath.Join(dir, "bin/generic-oauth-auth-provider") {
		t.Fatalf("expected generic auth provider command %q, got %q", filepath.Join(dir, "bin/generic-oauth-auth-provider"), provider.Spec.Command)
	}
	if len(provider.Spec.RequiredConfigurationParameters) != 6 {
		t.Fatalf("expected 6 required generic auth provider params, got %d", len(provider.Spec.RequiredConfigurationParameters))
	}
}
```

- [ ] **Step 3: Run the registry loader test**

Run:

```bash
go test ./pkg/controller/handlers/provider -run TestReadLocalProviderRegistryFromSubdirectories -count=1
```

Expected: test passes and proves Obot loads the generic provider manifest.

---

### Task 3: Pass Issuer and Email Verification Through Auth State

**Files:**
- Modify: `pkg/auth/auth.go`
- Modify: `pkg/proxy/proxy.go`
- Modify: `pkg/gateway/client/auth.go`
- Modify: `pkg/gateway/types/identity.go`
- Test: add `pkg/proxy/proxy_test.go` or extend existing proxy tests if present

- [ ] **Step 1: Extend serializable auth state types**

In `pkg/auth/auth.go`, add fields:

```go
type SerializableState struct {
	ExpiresOn         *time.Time `json:"expiresOn"`
	AccessToken       string     `json:"accessToken"`
	PreferredUsername string     `json:"preferredUsername"`
	User              string     `json:"user"`
	Email             string     `json:"email"`
	Issuer            string     `json:"issuer,omitempty"`
	EmailVerified     *bool      `json:"emailVerified,omitempty"`
	SetCookies        []string   `json:"setCookies"`
}
```

In `pkg/proxy/proxy.go`, add the same fields to the local `serializableState` struct:

```go
Issuer        string `json:"issuer,omitempty"`
EmailVerified *bool  `json:"emailVerified,omitempty"`
```

- [ ] **Step 2: Pass metadata through proxy extras**

In `pkg/proxy/proxy.go`, after constructing `u := &user.DefaultInfo{...}`, add:

```go
if ss.Issuer != "" {
	u.Extra["auth_provider_issuer"] = []string{ss.Issuer}
}
if ss.EmailVerified != nil {
	u.Extra["auth_provider_email_verified"] = []string{strconv.FormatBool(*ss.EmailVerified)}
}
```

Add the import:

```go
import "strconv"
```

- [ ] **Step 3: Extend identity type with non-primary metadata**

In `pkg/gateway/types/identity.go`, add non-primary fields:

```go
ProviderIssuer        string `json:"providerIssuer"`
ProviderEmailVerified *bool  `json:"providerEmailVerified" gorm:"-"`
```

`ProviderIssuer` is persisted for audit/debugging. The issuer-bound provider UID remains the uniqueness mechanism.

- [ ] **Step 4: Copy extras into gateway identity creation**

In `pkg/gateway/client/auth.go`, add:

```go
var emailVerified *bool
if raw := auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_email_verified"); raw != "" {
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, false, fmt.Errorf("invalid auth_provider_email_verified value %q: %w", raw, err)
	}
	emailVerified = &parsed
}
```

Add `strconv` to imports. Then set:

```go
identity := &types.Identity{
	Email:                 auth.FirstExtraValue(resp.User.GetExtra(), "email"),
	AuthProviderName:      auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_name"),
	AuthProviderNamespace: auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_namespace"),
	ProviderUsername:      resp.User.GetName(),
	ProviderUserID:        resp.User.GetUID(),
	ProviderIssuer:        auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_issuer"),
	ProviderEmailVerified: emailVerified,
}
```

- [ ] **Step 5: Run focused compile/tests**

Run:

```bash
go test ./pkg/proxy ./pkg/gateway/client -run 'TestNonExistent' -count=1
```

Expected: packages compile. It is acceptable for no tests to run if the packages compile.

---

### Task 4: Implement Generic Trusted Email Linking Rules

**Files:**
- Modify: `pkg/gateway/client/identity.go`
- Test: create `pkg/gateway/client/identity_generic_oauth_test.go`

- [ ] **Step 1: Add provider constants and helper functions**

In `pkg/gateway/client/identity.go`, add constants near `verifiedAuthProviders`:

```go
const (
	genericOAuthAuthProviderName = "generic-oauth-auth-provider"
	trustEmailLinkingEnvVar     = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING"
)
```

Replace direct `verified := slices.Contains(...)` with:

```go
verified := c.identityCanLinkByVerifiedEmail(ctx, id)
```

Add helper:

```go
func (c *Client) identityCanLinkByVerifiedEmail(ctx context.Context, id *types.Identity) bool {
	if slices.Contains(verifiedAuthProviders, fmt.Sprintf("%s/%s", id.AuthProviderNamespace, id.AuthProviderName)) {
		return true
	}

	if id.AuthProviderNamespace != system.DefaultNamespace || id.AuthProviderName != genericOAuthAuthProviderName {
		return false
	}

	if id.ProviderEmailVerified != nil && !*id.ProviderEmailVerified {
		return false
	}

	if id.ProviderIssuer == "" {
		return false
	}

	return c.genericOAuthTrustEmailLinking(ctx, id.ProviderIssuer)
}
```

Add:

```go
func (c *Client) genericOAuthTrustEmailLinking(ctx context.Context, issuer string) bool {
	var authProvider v1.AuthProvider
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: genericOAuthAuthProviderName}, &authProvider); err != nil {
		return false
	}

	cred, err := c.RevealCredential(ctx, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil {
		return false
	}

	if cred.Secrets[trustEmailLinkingEnvVar] == "" {
		return true
	}

	trusted, err := strconv.ParseBool(cred.Secrets[trustEmailLinkingEnvVar])
	return err == nil && trusted && cred.Secrets["OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER"] == issuer
}
```

Add imports for `strconv` and ensure `v1`, `system`, and `kclient` remain available.

- [ ] **Step 2: Preserve email verification update behavior**

Keep existing behavior:

```go
if user.VerifiedEmail == nil || (verified && !*user.VerifiedEmail) {
	user.VerifiedEmail = &verified
	userChanged = true
}
```

Do not change GitHub/Google behavior in this task.

- [ ] **Step 3: Write generic linking tests**

Create `pkg/gateway/client/identity_generic_oauth_test.go` with tests that set up a gateway client using the package's existing test helpers. Cover:

```go
func TestGenericOAuthLinksByEmailWhenIssuerTrustedAndEmailVerified(t *testing.T) {}
func TestGenericOAuthDoesNotLinkWhenEmailVerifiedFalse(t *testing.T) {}
func TestGenericOAuthDoesNotLinkWhenIssuerChanges(t *testing.T) {}
func TestGenericOAuthDoesNotLinkWhenTrustDisabled(t *testing.T) {}
func TestGenericOAuthLinksWhenEmailVerifiedAbsentAndTrustEnabled(t *testing.T) {}
```

Each test should:

1. Create an existing verified user with `alice@example.com`.
2. Configure generic OAuth credential secrets with issuer and trust value.
3. Call `EnsureIdentity` with generic provider namespace/name, issuer, email verification value, and an issuer-bound provider UID.
4. Assert whether the returned user ID matches the existing user.

- [ ] **Step 4: Run identity tests**

Run:

```bash
go test ./pkg/gateway/client -run 'TestGenericOAuth|TestEnsureIdentity' -count=1
```

Expected: generic OAuth tests pass and existing identity tests still pass.

---

### Task 5: Validate Generic Provider Configuration

**Files:**
- Modify: `pkg/api/handlers/authprovider.go`
- Create: `pkg/api/handlers/generic_oauth_validation.go`
- Test: add/extend `pkg/api/handlers/authprovider_test.go`

- [ ] **Step 1: Add generic validation helper**

Create `pkg/api/handlers/generic_oauth_validation.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	GenericOAuthAuthProviderName        = "generic-oauth-auth-provider"
	GenericOAuthIssuerEnvVar           = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER"
	GenericOAuthEmailDomainsEnvVar     = "OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"
	GenericOAuthTrustEmailLinkingEnvVar = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING"
)

type oidcDiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

func validateGenericOAuthConfig(ctx context.Context, providerName string, envVars map[string]string) error {
	if providerName != GenericOAuthAuthProviderName {
		return nil
	}

	issuer := strings.TrimRight(strings.TrimSpace(envVars[GenericOAuthIssuerEnvVar]), "/")
	if issuer == "" {
		return fmt.Errorf("%s is required", GenericOAuthIssuerEnvVar)
	}
	if strings.TrimSpace(envVars[GenericOAuthEmailDomainsEnvVar]) == "" {
		return fmt.Errorf("%s is required", GenericOAuthEmailDomainsEnvVar)
	}

	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be a valid URL", GenericOAuthIssuerEnvVar)
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return fmt.Errorf("%s must use https", GenericOAuthIssuerEnvVar)
	}

	discoveryURL := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	var doc oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to parse OIDC discovery document: %w", err)
	}

	if strings.TrimRight(doc.Issuer, "/") != issuer {
		return fmt.Errorf("OIDC discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.JWKSURI == "" {
		return fmt.Errorf("OIDC discovery document is missing required endpoints")
	}

	envVars[GenericOAuthIssuerEnvVar] = issuer
	return nil
}
```

- [ ] **Step 2: Call validation from Configure**

In `pkg/api/handlers/authprovider.go`, after deleting empty env vars and before `UpsertCredential`, add:

```go
if err := validateGenericOAuthConfig(req.Context(), authProvider.Name, envVars); err != nil {
	return types.NewErrBadRequest("invalid generic OAuth provider configuration: %v", err)
}
```

- [ ] **Step 3: Add validation tests**

Add tests for:

```go
func TestValidateGenericOAuthConfigRejectsMissingIssuer(t *testing.T) {}
func TestValidateGenericOAuthConfigRejectsMissingEmailDomains(t *testing.T) {}
func TestValidateGenericOAuthConfigRejectsIssuerMismatch(t *testing.T) {}
func TestValidateGenericOAuthConfigAcceptsValidDiscovery(t *testing.T) {}
```

Use `httptest.Server` with a discovery response:

```json
{
  "issuer": "http://127.0.0.1:PORT",
  "authorization_endpoint": "http://127.0.0.1:PORT/auth",
  "token_endpoint": "http://127.0.0.1:PORT/token",
  "jwks_uri": "http://127.0.0.1:PORT/jwks",
  "userinfo_endpoint": "http://127.0.0.1:PORT/userinfo"
}
```

- [ ] **Step 4: Run validation tests**

Run:

```bash
go test ./pkg/api/handlers -run 'TestValidateGenericOAuthConfig' -count=1
```

Expected: validation tests pass.

---

### Task 6: Handle Issuer Changes and Trust Reconfirmation

**Files:**
- Modify: `pkg/api/handlers/authprovider.go`
- Modify: `pkg/api/handlers/generic_oauth_validation.go`
- Test: `pkg/api/handlers/authprovider_test.go`

- [ ] **Step 1: Detect prior issuer before storing new credentials**

In `Configure`, before `UpsertCredential`, reveal existing generic provider credential if present:

```go
existingIssuer := ""
if authProvider.Name == GenericOAuthAuthProviderName {
	if existing, err := req.GatewayClient.RevealCredential(req.Context(), []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name); err == nil && existing.Secrets != nil {
		existingIssuer = existing.Secrets[GenericOAuthIssuerEnvVar]
	}
}
```

- [ ] **Step 2: Require trust reconfirmation when issuer changes**

Add helper:

```go
func requireGenericOAuthTrustReconfirmation(providerName, existingIssuer string, envVars map[string]string) error {
	if providerName != GenericOAuthAuthProviderName || existingIssuer == "" {
		return nil
	}
	newIssuer := envVars[GenericOAuthIssuerEnvVar]
	if newIssuer == existingIssuer {
		return nil
	}
	if envVars[GenericOAuthTrustEmailLinkingEnvVar] == "true" {
		return nil
	}
	return fmt.Errorf("issuer changed from %q to %q; account-linking trust must be re-confirmed", existingIssuer, newIssuer)
}
```

Call it after validation canonicalizes the issuer.

- [ ] **Step 3: Stop provider daemon on issuer change**

The existing `Configure` already calls:

```go
ap.dispatcher.StopAuthProvider(authProvider.Namespace, authProvider.Name)
```

Keep this behavior. Add a test assertion with a fake dispatcher only if existing handler tests already use one; otherwise cover the behavior through code review and integration tests.

- [ ] **Step 4: Add tests**

Cover:

```go
func TestGenericOAuthIssuerChangeRequiresTrustReconfirmation(t *testing.T) {}
func TestGenericOAuthIssuerChangeAllowsExplicitTrust(t *testing.T) {}
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./pkg/api/handlers -run 'TestGenericOAuthIssuerChange|TestValidateGenericOAuthConfig' -count=1
```

Expected: tests pass.

---

### Task 7: Add UI Support for Generic Provider Trust

**Files:**
- Modify: `ui/user/src/routes/admin/auth-providers/+page.svelte`
- Modify: `ui/user/src/lib/components/admin/ProviderConfigure.svelte`
- Modify: `ui/user/src/lib/services/admin/types.ts` only if needed
- Test: existing UI check/lint suite

- [ ] **Step 1: Add generic provider to preferred sort order**

In `ui/user/src/routes/admin/auth-providers/+page.svelte`, update:

```ts
const preferredOrder: string[] = [
	CommonAuthProviderIds.GOOGLE,
	CommonAuthProviderIds.GITHUB,
	'generic-oauth-auth-provider',
	CommonAuthProviderIds.OKTA,
	CommonAuthProviderIds.AUTH0
];
```

- [ ] **Step 2: Initialize generic defaults on first configure**

In the 404 reveal path where default values are set, use:

```ts
configuringAuthProviderValues = {
	OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*',
	...(authProvider.id === 'generic-oauth-auth-provider'
		? {
				OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE: 'openid email profile',
				OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING: 'true'
			}
		: {})
};
```

- [ ] **Step 3: Render trust setting as a toggle**

In `ui/user/src/lib/components/admin/ProviderConfigure.svelte`, add the generic trust env var to `booleanInputs`:

```ts
const booleanInputs = new Set([
	'OBOT_AUTH_PROVIDER_ENABLE_LOGGING',
	'OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING'
]);
```

- [ ] **Step 4: Render issuer warning in generic configure dialog**

In `ProviderConfigure.svelte`, below the parameter list and only when `provider?.id === 'generic-oauth-auth-provider'`, render:

```svelte
{#if provider?.id === 'generic-oauth-auth-provider'}
	<div class="notification-info p-3 text-sm font-light">
		Changing the issuer changes the identity trust boundary. Re-confirm account linking only
		when you trust the new issuer to own the email addresses it returns.
	</div>
{/if}
```

- [ ] **Step 5: Run UI checks**

Run:

```bash
cd ui/user
pnpm run check
pnpm run lint
```

Expected: both commands pass.

---

### Task 8: Add User Profile Mapping for Generic Provider

**Files:**
- Modify: `pkg/gateway/client/user.go`
- Test: add or extend user profile tests if package fixtures exist

- [ ] **Step 1: Add generic provider case**

In `pkg/gateway/client/user.go`, add a switch case:

```go
case "generic-oauth-auth-provider":
	if iconURL, ok := profile["picture"].(string); ok {
		user.IconURL = iconURL
		identity.IconURL = iconURL
	}
	if displayName, ok := profile["preferred_username"].(string); ok && displayName != "" {
		user.DisplayName = displayName
	} else if displayName, ok := profile["name"].(string); ok && displayName != "" {
		user.DisplayName = displayName
	} else if email, ok := profile["email"].(string); ok {
		user.DisplayName = email
	}
```

- [ ] **Step 2: Run gateway client tests**

Run:

```bash
go test ./pkg/gateway/client -run 'Test.*User|Test.*Identity|Test.*Session' -count=1
```

Expected: tests pass.

---

### Task 9: Document Generic OAuth Provider Setup

**Files:**
- Modify: `docs/docs/configuration/auth-providers.md`

- [ ] **Step 1: Add generic OAuth section**

Insert a section under Available Auth Providers:

```markdown
### Custom OAuth / OIDC

Use Custom OAuth / OIDC when your identity provider supports OpenID Connect discovery.
Obot supports one configured custom provider at a time, following the same one-provider
configuration model as the other auth providers.

Required fields:

| Obot | Meaning |
|------|---------|
| Provider Name | Login button label, such as `Studio` or `Acme SSO`. |
| Issuer URL | OIDC issuer URL. The issuer must expose `/.well-known/openid-configuration`. |
| Client ID | OAuth/OIDC client ID. |
| Client Secret | OAuth/OIDC client secret. |
| Email Domains | Comma-separated allowed domains, or `*` to allow every domain trusted by the issuer. |
| Trust this issuer for account linking | Allows this issuer to link logins to existing Obot users by email. |

Optional fields:

| Obot | Default |
|------|---------|
| Scope | `openid email profile` |

Add the callback URL shown in Obot's provider configuration dialog to the identity
provider's allowed redirect URI list.

Account linking trust is scoped to the configured issuer. If you change the issuer,
Obot treats it as a new identity trust boundary and requires account-linking trust
to be re-confirmed.

If the provider returns `email_verified=false`, Obot does not link by email. If the
claim is absent, Obot relies on the admin's issuer trust setting.

Example issuer URL formats:

- Entra: `https://login.microsoftonline.com/<tenant-id>/v2.0`
- Keycloak: `https://keycloak.example.com/realms/<realm>`
- Okta: `https://<your-okta-domain>`
- Studio: use the issuer URL published by the Studio deployment
```

- [ ] **Step 2: Run docs verification**

Run the repo's docs command:

```bash
cd docs
pnpm install
pnpm run build
```

Expected: Docusaurus build passes.

---

### Task 10: Add Full OAuth/OIDC Browser E2E Coverage

**Files:**
- Create: `ui/user/playwright.config.ts` or repo-level `playwright.config.ts`
- Modify: `ui/user/package.json` or repo-level package/test scripts
- Create: `ui/user/e2e/generic-oauth.spec.ts` or `e2e/generic-oauth.spec.ts`
- Create: `ui/user/e2e/fixtures/generic-oauth.mock.json` or `e2e/fixtures/generic-oauth.mock.json`
- Create: `ui/user/e2e/fixtures/keycloak.settings.json` or `e2e/fixtures/keycloak.settings.json`
- Create: `ui/user/e2e/fixtures/keycloak.realm.json` or `e2e/fixtures/keycloak.realm.json`
- Create: `ui/user/e2e/support/obot.ts` or `e2e/support/obot.ts`
- Create: `ui/user/e2e/support/mock-oidc.ts` or `e2e/support/mock-oidc.ts`
- Create: `ui/user/e2e/support/keycloak.ts` or `e2e/support/keycloak.ts`

- [ ] **Step 1: Add fixture files for all test settings**

Create `generic-oauth.mock.json`:

```json
{
  "providerName": "Mock OIDC",
  "clientId": "obot",
  "clientSecret": "obot-secret",
  "scope": "openid email profile",
  "emailDomains": "*",
  "trustEmailLinking": true,
  "user": {
    "sub": "test-user-1",
    "email": "oauth-user@example.com",
    "emailVerified": true,
    "preferredUsername": "oauth-user",
    "name": "OAuth User",
    "picture": "https://example.com/oauth-user.png"
  }
}
```

Create `keycloak.settings.json`:

```json
{
  "realm": "obot-test",
  "providerName": "Keycloak OIDC",
  "clientId": "obot",
  "clientSecret": "obot-secret",
  "scope": "openid email profile",
  "emailDomains": "*",
  "trustEmailLinking": true,
  "username": "oauth-user@example.com",
  "password": "Password123!",
  "email": "oauth-user@example.com",
  "preferredUsername": "oauth-user",
  "name": "OAuth User"
}
```

Create `keycloak.realm.json` from the same values. The realm import must define:

```json
{
  "realm": "obot-test",
  "enabled": true,
  "clients": [
    {
      "clientId": "obot",
      "secret": "obot-secret",
      "enabled": true,
      "protocol": "openid-connect",
      "publicClient": false,
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": false,
      "redirectUris": ["http://localhost:*/oauth2/callback"],
      "webOrigins": ["http://localhost:*"]
    }
  ],
  "users": [
    {
      "username": "oauth-user@example.com",
      "email": "oauth-user@example.com",
      "emailVerified": true,
      "enabled": true,
      "firstName": "OAuth",
      "lastName": "User",
      "credentials": [
        {
          "type": "password",
          "value": "Password123!",
          "temporary": false
        }
      ]
    }
  ]
}
```

Do not put provider/client/user values directly in test code. Test helpers must load these fixture files.

- [ ] **Step 2: Add Playwright and OIDC test dependencies**

Add dev dependencies to the package that owns the E2E suite:

```bash
pnpm add -D @playwright/test oauth2-mock-server
pnpm exec playwright install chromium
```

Add scripts:

```json
{
  "scripts": {
    "e2e": "playwright test",
    "e2e:generic-oauth": "playwright test generic-oauth.spec.ts"
  }
}
```

- [ ] **Step 3: Add mock OIDC support helper**

Create `mock-oidc.ts` with a helper that:

1. Loads `generic-oauth.mock.json`.
2. Starts `oauth2-mock-server` on an available local port.
3. Generates an RSA signing key.
4. Configures token/userinfo claims from the fixture.
5. Returns:

```ts
type MockOIDCServer = {
  providerName: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  scope: string;
  emailDomains: string;
  trustEmailLinking: boolean;
  user: {
    sub: string;
    email: string;
    emailVerified: boolean;
    preferredUsername: string;
    name: string;
  };
  stop(): Promise<void>;
};
```

The mock server must emit:

```json
{
  "sub": "test-user-1",
  "email": "oauth-user@example.com",
  "email_verified": true,
  "preferred_username": "oauth-user",
  "name": "OAuth User"
}
```

Expected: Obot's generic provider can discover `/.well-known/openid-configuration`, fetch JWKS, exchange the code, validate the ID token, and call `/userinfo`.

- [ ] **Step 4: Add Keycloak support helper**

Create `keycloak.ts` with a helper that:

1. Returns early with a skipped-test marker when `OBOT_E2E_KEYCLOAK=false`.
2. Starts a local Keycloak container when `OBOT_E2E_KEYCLOAK` is unset or any value other than `false`.
3. Imports `keycloak.realm.json`.
4. Waits for `/.well-known/openid-configuration` under `/realms/obot-test`.
5. Returns:

```ts
type KeycloakServer = {
  providerName: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  scope: string;
  emailDomains: string;
  trustEmailLinking: boolean;
  username: string;
  password: string;
  stop(): Promise<void>;
};
```

The only environment variable allowed for this lane is:

```bash
OBOT_E2E_KEYCLOAK=false
```

CI must set `OBOT_E2E_KEYCLOAK=false`. Local runs default to running Keycloak and require Docker. If Docker is unavailable, fail with a message that tells the developer to start Docker or run with `OBOT_E2E_KEYCLOAK=false`.

- [ ] **Step 5: Add Obot E2E harness helper**

Create `obot.ts` with helpers that:

1. Start Obot with authentication enabled and a deterministic bootstrap token.
2. Start the user UI if the E2E suite runs against Vite rather than the Go server's static UI.
3. Point `OBOT_SERVER_PROVIDER_REGISTRIES` at a fixture provider registry containing `generic-oauth-auth-provider`.
4. Configure the generic provider through the public API or admin UI using fixture values.
5. Expose:

```ts
type ObotE2E = {
  baseURL: string;
  bootstrapToken: string;
  configureGenericProvider(input: {
    providerName: string;
    issuer: string;
    clientId: string;
    clientSecret: string;
    scope: string;
    emailDomains: string;
    trustEmailLinking: boolean;
  }): Promise<void>;
  stop(): Promise<void>;
};
```

Do not bypass the provider configuration API for the main assertions. The test must prove that first-class generic provider configuration works.

- [ ] **Step 6: Write the failing Playwright test for the CI-safe mock OIDC flow**

Create `generic-oauth.spec.ts` with:

```ts
test('admin configures generic OAuth and user logs in through mock OIDC', async ({ page }) => {
  const oidc = await startMockOIDCFromFixture();
  const obot = await startObotE2E();

  await obot.configureGenericProvider({
    providerName: oidc.providerName,
    issuer: oidc.issuer,
    clientId: oidc.clientId,
    clientSecret: oidc.clientSecret,
    scope: oidc.scope,
    emailDomains: oidc.emailDomains,
    trustEmailLinking: oidc.trustEmailLinking
  });

  await page.goto(obot.baseURL);
  await page.getByRole('button', { name: /mock oidc/i }).click();
  await expect(page).toHaveURL(/oauth2\/callback|\/$/);
  await expect(page.getByText('oauth-user@example.com')).toBeVisible();

  await obot.stop();
  await oidc.stop();
});
```

Run:

```bash
pnpm run e2e:generic-oauth -- --grep "mock OIDC"
```

Expected before implementation: FAIL because the harness/test does not exist.

- [ ] **Step 7: Implement the mock OIDC flow**

Implement the helpers and test from Steps 3, 5, and 6. The browser must travel through:

```text
Obot login page
  -> /oauth2/start
  -> oauth2-mock-server /authorize
  -> Obot /oauth2/callback
  -> authenticated Obot UI
```

Expected after implementation: mock OIDC Playwright test passes in local runs and CI.

- [ ] **Step 8: Write the failing Playwright test for the Keycloak credential flow**

Add a second test:

```ts
test('user logs in through Keycloak username/password form', async ({ page }) => {
  test.skip(process.env.OBOT_E2E_KEYCLOAK === 'false', 'Keycloak credential flow disabled for CI');

  const keycloak = await startKeycloakFromFixture();
  const obot = await startObotE2E();

  await obot.configureGenericProvider({
    providerName: keycloak.providerName,
    issuer: keycloak.issuer,
    clientId: keycloak.clientId,
    clientSecret: keycloak.clientSecret,
    scope: keycloak.scope,
    emailDomains: keycloak.emailDomains,
    trustEmailLinking: keycloak.trustEmailLinking
  });

  await page.goto(obot.baseURL);
  await page.getByRole('button', { name: /keycloak oidc/i }).click();
  await page.getByLabel(/username|email/i).fill(keycloak.username);
  await page.getByLabel(/password/i).fill(keycloak.password);
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page.getByText('oauth-user@example.com')).toBeVisible();

  await obot.stop();
  await keycloak.stop();
});
```

Run:

```bash
OBOT_E2E_KEYCLOAK=false pnpm run e2e:generic-oauth -- --grep "Keycloak"
```

Expected: SKIP.

Run locally with Docker:

```bash
pnpm run e2e:generic-oauth -- --grep "Keycloak"
```

Expected before implementation: FAIL because Keycloak helper does not exist.

- [ ] **Step 9: Implement the Keycloak credential flow**

Implement the Keycloak helper using the fixture realm import. The test must fill username and password in the Keycloak login page rather than bypassing authentication by API.

Expected after implementation:

```bash
OBOT_E2E_KEYCLOAK=false pnpm run e2e:generic-oauth
```

passes the mock OIDC test and skips Keycloak.

```bash
pnpm run e2e:generic-oauth
```

passes both mock OIDC and Keycloak tests when Docker is running.

- [ ] **Step 10: Add CI configuration**

Update CI to run:

```bash
OBOT_E2E_KEYCLOAK=false pnpm run e2e:generic-oauth
```

Expected: CI runs the mock OIDC full redirect flow and skips the Keycloak credential-flow lane.

- [ ] **Step 11: Commit**

Run:

```bash
git add \
  ui/user/package.json \
  ui/user/pnpm-lock.yaml \
  ui/user/playwright.config.ts \
  ui/user/e2e \
  .github/workflows
git commit -m "test: add generic oauth browser e2e"
```

Expected: commit succeeds with Playwright tests, fixtures, and CI wiring.

---

### Task 11: Full Verification and Commit

**Files:**
- All modified files from prior tasks

- [ ] **Step 1: Run Go tests for touched packages**

Run:

```bash
go test ./pkg/controller/handlers/provider ./pkg/api/handlers ./pkg/proxy ./pkg/gateway/client -count=1
```

Expected: all packages pass.

- [ ] **Step 2: Run UI checks**

Run:

```bash
cd ui/user
pnpm run check
pnpm run lint
```

Expected: both pass.

- [ ] **Step 3: Run docs build**

Run:

```bash
cd docs
pnpm run build
```

Expected: build passes.

- [ ] **Step 4: Run generic OAuth E2E tests**

Run:

```bash
cd ui/user
OBOT_E2E_KEYCLOAK=false pnpm run e2e:generic-oauth
```

Expected: mock OIDC full-flow test passes and Keycloak credential-flow test is skipped.

Run locally with Docker:

```bash
cd ui/user
pnpm run e2e:generic-oauth
```

Expected: mock OIDC full-flow test passes and Keycloak credential-flow test passes.

- [ ] **Step 5: Review diff**

Run:

```bash
git status --short
git diff --stat
git diff -- docs/design/generic-oauth-provider/README.md docs/plans/2026-06-10-generic-oauth-provider-support.md
```

Expected: diff contains the generic OAuth provider implementation, docs, and E2E fixtures/tests only.

- [ ] **Step 6: Commit**

Run:

```bash
git add \
  docs/design/generic-oauth-provider/README.md \
  docs/plans/2026-06-10-generic-oauth-provider-support.md \
  docs/docs/configuration/auth-providers.md \
  apiclient/types/authprovider.go \
  pkg/auth/auth.go \
  pkg/proxy/proxy.go \
  pkg/gateway/types/identity.go \
  pkg/gateway/client/auth.go \
  pkg/gateway/client/identity.go \
  pkg/gateway/client/user.go \
  pkg/api/handlers/authprovider.go \
  pkg/api/handlers/generic_oauth_validation.go \
  pkg/controller/handlers/provider/provider.go \
  ui/user/src/routes/admin/auth-providers/+page.svelte \
  ui/user/src/lib/components/admin/ProviderConfigure.svelte \
  ui/user/src/lib/services/admin/types.ts \
  ui/user/playwright.config.ts \
  ui/user/e2e \
  ui/user/package.json \
  ui/user/pnpm-lock.yaml
git commit -m "feat: add generic oauth provider support"
```

Expected: commit succeeds.

---

## Self-Review Notes

- The plan defers group mapping in v1, matching the design discussion.
- The plan chooses issuer-bound provider UID instead of adding a required identity primary-key migration.
- The plan makes trusted email linking explicit provider configuration and issuer-scoped.
- The plan keeps the current one-configured-provider rule.
- The plan identifies the provider image/repo dependency as Task 1 because Obot cannot enable the registry entry safely without the daemon contract.
- The plan adds CI-safe Playwright coverage through `oauth2-mock-server`.
- The plan adds opt-in Keycloak credential-flow coverage that runs locally by default and is disabled in CI with `OBOT_E2E_KEYCLOAK=false`.
- The plan requires all E2E provider/client/user settings to live in fixture files.
