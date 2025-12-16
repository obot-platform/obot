# Upstream Sync Analysis Report

**Date:** December 15, 2025
**Fork:** `jrmatherly/obot-entraid`
**Upstream:** `obot-platform/obot`
**Merge Base:** `b7ac8670`

---

## Executive Summary

The fork is **32 commits behind upstream** and has **20 local commits** containing custom features (EntraID/Keycloak auth providers, branding preferences).

### Merge Complexity Assessment

| Category | Count | Effort |
|----------|-------|--------|
| Git conflicts (trivial) | 2 files | ~5 minutes |
| Manual merge required | 9 files | ~2-3 hours |
| Keep our version | 4 files | ~15 minutes (verify only) |
| Auto-merge safe | ~180 files | Automatic |
| Regenerate after merge | 2 files | ~5 minutes |

### Key Challenges

1. **UI Component Merges** - Upstream's Svelte 5 `untrack()` pattern needs to be adopted while preserving our branding UI additions
2. **Store Logic Integration** - Our `appPreferences` store has branding logic that must be carefully merged with upstream's structure
3. **Helm Chart Reconciliation** - Multiple config values differ; must selectively update while preserving fork-specific settings
4. **Upstream Bug Avoidance** - Upstream introduced a bug (hardcoded "Google" in auth-providers page) that we must NOT adopt

### What Must Be Preserved
- Custom auth providers (`tools/entra-auth-provider/`, `tools/keycloak-auth-provider/`)
- `BrandingPreferences` feature (types, storage, UI)
- Custom Dockerfile with auth provider build stages
- Fork-specific Helm chart configuration
- Custom CI/CD workflows

**Estimated Total Effort:** 3-4 hours for careful merge + testing

---

## Branch Status

### Upstream Commits Not in Our Main (32 commits)
Key upstream changes since our last merge:

| Category | Commits | Impact |
|----------|---------|--------|
| UI/UX Overhaul | 3 (#5180, #5297, #5309, #5324) | **High** - Major route restructuring |
| MCP Improvements | 8+ | Medium - MCP server/registry enhancements |
| Bug Fixes | 15+ | Low - Various fixes |
| Security Updates | 2 (#5233, #5308) | Medium - crypto library updates |
| Nanobot Bumps | 2 (#5268, #5325) | Low - Image version updates |

### Our Commits Not in Upstream (20 commits)
Our custom features that must be preserved:

| Feature | Files | Status |
|---------|-------|--------|
| EntraID Auth Provider | `tools/entra-auth-provider/*` | Must preserve |
| Keycloak Auth Provider | `tools/keycloak-auth-provider/*` | Must preserve |
| Auth Providers Common | `tools/auth-providers-common/*` | Must preserve |
| Branding Preferences | `apiclient/types/apppreferences.go`, UI files | Must preserve |
| Custom Dockerfile | `Dockerfile` | Must preserve |
| Custom Helm Chart | `chart/*` | Must preserve |
| Custom CI/CD | `.github/workflows/*` | Must preserve |

---

## Conflict Analysis

### Actual Git Conflicts (2 files)

#### 1. `.gitignore` - **Trivial**
```diff
<<<<<<< HEAD (Our changes)
.serena/
.archive/
.cloned-obot-tools/
*.tgz
=======
thoughts
>>>>>>> upstream/main
```
**Resolution:** Keep both - merge all entries. Our entries are additive.

#### 2. `logger/go.mod` - **Trivial**
```diff
<<<<<<< HEAD
go 1.25.5
=======
go 1.23.5
>>>>>>> upstream/main
```
**Resolution:** Keep our `go 1.25.5` for consistency. Upstream has an inconsistency where `logger/go.mod` uses `1.23.5` while their main `go.mod` and `apiclient/go.mod` both use `1.25.5`. Our fork correctly uses `1.25.5` across all modules.

### Structural Differences (Not Conflicts)

These files show as changes because they exist in our fork but not upstream:

| File/Directory | Status | Action Required |
|----------------|--------|-----------------|
| `tools/entra-auth-provider/*` | Our addition | No action - keep |
| `tools/keycloak-auth-provider/*` | Our addition | No action - keep |
| `tools/auth-providers-common/*` | Our addition | No action - keep |
| `tools/index.yaml` | Our addition | No action - keep |
| `tools/placeholder-credential/*` | Our addition | No action - keep |
| `tools/tool.gpt` | Our addition | No action - keep |
| `docs/docs/configuration/entra-id-authentication.md` | Upstream deleted | Keep ours |
| `docs/docs/configuration/app-preferences.md` | Upstream deleted | Keep ours |
| `docs/docs/contributing/upstream-merge-process.md` | Upstream deleted | Keep ours |

---

## File Categories

### Category A: Safe to Auto-Merge (200+ files)
Files with changes only in upstream that don't conflict with our customizations:

- `pkg/api/handlers/*.go` (except apppreferences.go)
- `pkg/mcp/*.go`
- `pkg/controller/handlers/*`
- `ui/user/src/lib/components/*.svelte` (most)
- `ui/user/src/routes/admin/*.svelte` (most)
- `docs/docs/*.md` (most)

### Category B: Requires Manual Review (15 files)
Files we both modified:

| File | Our Changes | Upstream Changes | Risk |
|------|-------------|------------------|------|
| `apiclient/types/apppreferences.go` | Added `BrandingPreferences` | No change | **Low** - additive |
| `apiclient/types/zz_generated.deepcopy.go` | Regenerated with Branding | No change | **Low** - regenerate after merge |
| `pkg/storage/apis/.../apppreferences.go` | Added `Branding` field | No change | **Low** - additive |
| `pkg/storage/openapi/generated/openapi_generated.go` | Updated for Branding | No change | **Low** - regenerate |
| `ui/user/src/routes/admin/app-preferences/+page.svelte` | Added Branding UI | Minor fixes | **Medium** - manual merge |
| `ui/user/src/lib/stores/appPreferences.svelte.ts` | Branding store logic | Store refactoring | **Medium** - manual merge |
| `Dockerfile` | Custom auth provider build | Minor updates | **Low** - keep ours |
| `chart/values.yaml` | Custom image, probes, config | nanobot bump | **Medium** - manual merge |
| `chart/Chart.yaml` | Our version numbers | No change | **Low** - keep ours |
| `.github/workflows/*.yml` | Custom CI/CD | Various updates | **Medium** - compare each |

### Category C: Our Custom Additions (Keep As-Is)
Files that only exist in our fork:

```
tools/
├── entra-auth-provider/
│   ├── main.go
│   ├── pkg/profile/profile.go
│   ├── tool.gpt
│   ├── go.mod, go.sum
│   └── Makefile
├── keycloak-auth-provider/
│   ├── main.go
│   ├── pkg/profile/profile.go
│   ├── tool.gpt
│   ├── go.mod, go.sum
│   └── Makefile
├── auth-providers-common/
│   ├── pkg/env/env.go
│   ├── pkg/icon/icon.go
│   ├── pkg/state/state.go
│   └── templates/error.html
├── placeholder-credential/
├── index.yaml
└── tool.gpt
```

---

## Detailed File-by-File Merge Guide

This section provides specific merge instructions for each Category B file requiring manual attention.

---

### 1. `ui/user/src/routes/admin/app-preferences/+page.svelte`

**Complexity:** Medium | **Risk:** Medium

#### What Upstream Changed
```diff
+ import { untrack } from 'svelte';
- let form = $state<AppPreferences>(data.appPreferences);
+ let form = $state<AppPreferences>(untrack(() => data.appPreferences));
- <div class="relative mt-4 h-full w-full">
+ <div class="relative h-full w-full">
```
- Added `untrack` import and usage (Svelte 5 reactivity fix)
- Removed `mt-4` margin from container div

#### What We Added
- 58-line Footer Branding section with:
  - Product Name input
  - Issue Report URL input
  - Footer Message input with `{productName}` placeholder support
  - Show Footer toggle

#### Merge Strategy
1. **Accept upstream's `untrack` changes** - this is a bug fix for Svelte 5 reactivity
2. **Accept upstream's layout change** (`mt-4` removal)
3. **Re-add our Footer Branding section** after the Theme Colors section, before the save button

#### Post-Merge Code Location
Insert our branding section at line ~293 (after Theme Colors `</div>`, before `{#if !isAdminReadonly}`):
```svelte
<div class="flex flex-col gap-1">
    <h2 class="text-lg font-semibold">Footer Branding</h2>
    <!-- ... our branding UI ... -->
</div>
```

---

### 2. `ui/user/src/lib/stores/appPreferences.svelte.ts`

**Complexity:** Medium | **Risk:** Medium

#### What Upstream Has
- Basic store with logos and theme defaults
- `compileAppPreferences()` function for logos and theme only
- No client-side initialization

#### What We Added
```typescript
// Our additions:
import { listAppPreferences } from '$lib/services/admin/operations';

export const DEFAULT_BRANDING = {
    productName: 'Obot',
    issueReportUrl: 'https://github.com/jrmatherly/obot-entraid/issues/new?template=bug_report.md',
    footerMessage: "{productName} isn't perfect. Double check its work.",
    showFooter: true
} as const;

// In compileAppPreferences():
branding: {
    productName: preferences?.branding?.productName ?? DEFAULT_BRANDING.productName,
    // ... etc
}

// Client-side init function
async function init() { ... }
if (browser) { init(); }
```

#### Merge Strategy
1. **Keep upstream's base structure**
2. **Add our imports** (`listAppPreferences`)
3. **Add our `DEFAULT_BRANDING` constant** after `DEFAULT_LOGOS`
4. **Extend `compileAppPreferences()`** to include branding section
5. **Add our `init()` function and browser initialization** at the end

#### Decision Point
Review whether our client-side `init()` is still needed or if upstream handles initialization differently now. Check `+layout.ts` or `+layout.svelte` for SSR data loading.

---

### 3. `chart/values.yaml`

**Complexity:** Medium | **Risk:** Medium

#### Line-by-Line Analysis

| Section | Our Value | Upstream Value | Resolution |
|---------|-----------|----------------|------------|
| `image.repository` | `ghcr.io/jrmatherly/obot-entraid` | `ghcr.io/obot-platform/obot` | **Keep ours** |
| `image.repository` comment | Multi-line with fork info | Single line | **Keep ours** |
| `OBOT_SERVER_MCPREMOTE_SHIM_BASE_IMAGE` | `nanobot:v0.0.44` | `nanobot:v0.0.45` | **Accept upstream** (version bump) |
| `OBOT_SERVER_DISABLE_UPDATE_CHECK` | `"true"` | `""` | **Keep ours** (fork needs this) |
| `OBOT_SERVER_TOOL_REGISTRIES` | `"/obot-tools/tools"` | (not present) | **Keep ours** (required for auth providers) |
| `mcpNamespace.create` | `true` | (not present) | **Keep ours** |
| `mcpNamespace` comment | Enhanced | Basic | **Keep ours** |
| `livenessProbe` | Custom config | (not present) | **Keep ours** |
| `readinessProbe` | Custom config | (not present) | **Keep ours** |

#### Merge Strategy
1. Start with **our values.yaml as base**
2. **Update only:** `OBOT_SERVER_MCPREMOTE_SHIM_BASE_IMAGE` to `v0.0.45`
3. Review any other upstream additions not listed above

---

### 4. `Dockerfile`

**Complexity:** Low | **Risk:** Low

#### What Upstream Changed
- Removed `package-chrome.sh` and chromium packaging
- Simplified tools copying (no `tools-patched` stage)
- Removed hadolint ignore comments

#### What We Have
- Custom `tools-patched` build stage
- EntraID and Keycloak auth provider compilation
- Index.yaml merging logic
- Chrome packaging (may no longer be needed?)

#### Merge Strategy
**Keep our Dockerfile entirely.** Our build process is fundamentally different:
1. We build custom auth providers
2. We merge tool registries
3. We set `OBOT_SERVER_TOOL_REGISTRIES`

#### Action Item
Investigate if we still need `package-chrome.sh`. If upstream removed it, we may be able to remove it too (reduces image size).

---

### 5. `.github/workflows/docker-build-and-push.yml`

**Complexity:** High | **Risk:** Low (separate CI system)

#### Key Differences

| Aspect | Our Workflow | Upstream Workflow |
|--------|--------------|-------------------|
| Runner | `ubuntu-latest` | `depot-ubuntu-22.04` |
| Build Tool | `docker/build-push-action` | `depot/build-push-action` |
| Auth | `github.actor` / `GITHUB_TOKEN` | `GHCR_USERNAME` / `GHCR_TOKEN` |
| Jobs | Single `build` job | Multiple: `oss-build`, `oss-image-scan`, `enterprise-build`, `enterprise-image-scan` |
| Platforms | `linux/amd64` only | `linux/amd64,linux/arm64` |
| Registry | GHCR only | GHCR + Docker Hub |
| Image Signing | Basic cosign | Enhanced cosign with crane tagging |

#### Merge Strategy
**Keep our workflow.** We have different:
- Publishing targets (our GHCR repo)
- Authentication setup
- No need for enterprise build
- No Depot subscription

#### Potential Improvements to Adopt
- Add `linux/arm64` platform support (if needed)
- Add Trivy security scanning job
- Consider image signing improvements

---

### 6. `ui/user/src/routes/admin/auth-providers/+page.svelte`

**Complexity:** Low | **Risk:** Low

#### What Upstream Changed
```diff
+ import { untrack } from 'svelte';
- let authProviders = $state(initialAuthProviders);
+ let authProviders = $state(untrack(() => data.authProviders));

// Added owner existence check in handleOwnerSetup:
+ const users = await AdminService.listUsers();
+ const isOwnerExist = users.some((user) => user.role === Role.OWNER);
+ if (isOwnerExist) return;

// Layout changes:
- <Layout>
+ <Layout title="Auth Providers">
- <div class="my-4"
+ <div class="mb-4"
- <div class="grid ... py-8
+ <div class="grid ... px-8
```

#### Upstream Bug Found
```diff
- Are you sure you want to deconfigure <b>{confirmDeconfigureAuthProvider?.name}</b>?
+ Are you sure you want to deconfigure <b>Google</b>?
```
Upstream hardcoded "Google" instead of using dynamic provider name. **We should NOT adopt this bug.**

#### Merge Strategy
1. **Accept** the `untrack` pattern
2. **Accept** the `handleOwnerSetup` owner existence check (good improvement)
3. **Accept** the Layout title and margin changes
4. **REJECT** the hardcoded "Google" - keep our dynamic `{confirmDeconfigureAuthProvider?.name}`

---

### 7. `apiclient/types/apppreferences.go`

**Complexity:** Low | **Risk:** Low

#### Our Addition
```go
type AppPreferences struct {
    Logos    LogoPreferences     `json:"logos,omitempty"`
    Theme    ThemePreferences    `json:"theme,omitempty"`
    Branding BrandingPreferences `json:"branding,omitempty"`  // OUR ADDITION
    Metadata Metadata            `json:"metadata,omitempty"`
}

// OUR ADDITION:
type BrandingPreferences struct {
    ProductName    string `json:"productName,omitempty"`
    IssueReportURL string `json:"issueReportUrl,omitempty"`
    FooterMessage  string `json:"footerMessage,omitempty"`
    ShowFooter     *bool  `json:"showFooter,omitempty"`
}
```

#### Merge Strategy
**Keep our version.** This is purely additive. Upstream has no changes to this file.

---

### 8. `pkg/storage/apis/obot.obot.ai/v1/apppreferences.go`

**Complexity:** Low | **Risk:** Low

#### Our Addition
```go
type AppPreferencesSpec struct {
    Logos    types.LogoPreferences     `json:"logos,omitempty"`
    Theme    types.ThemePreferences    `json:"theme,omitempty"`
    Branding types.BrandingPreferences `json:"branding,omitempty"`  // OUR ADDITION
}
```

#### Merge Strategy
**Keep our version.** Purely additive.

---

### 9. Generated Files (Regenerate After Merge)

These files should be regenerated, not manually merged:

| File | Regeneration Command |
|------|---------------------|
| `apiclient/types/zz_generated.deepcopy.go` | `make generate` |
| `pkg/storage/openapi/generated/openapi_generated.go` | `make generate` |

**Important:** Run `make generate` AFTER resolving all other conflicts to ensure these files include our `BrandingPreferences` type.

---

## Major Upstream Changes to Understand

### 1. UI/UX Overhaul (PR #5180)
**Impact:** High - Route restructuring

Changes:
- `mcp-publisher/` routes merged into `mcp-servers/`
- `access-control/` renamed to `mcp-registries/`
- New `details/` routes added under MCP server instances
- New store `mcpServersAndEntries.svelte.ts` replaces context pattern
- `RegistriesView.svelte` removed (functionality moved elsewhere)

**Our Impact:** Our `app-preferences/+page.svelte` has branding additions that need to be carefully merged with upstream's minor layout fixes.

### 2. Nanobot Image Bumps
```yaml
# Upstream bumped from v0.0.44 to v0.0.45
OBOT_SERVER_MCPREMOTE_SHIM_BASE_IMAGE: "ghcr.io/nanobot-ai/nanobot:v0.0.45"
```
**Action:** Accept this bump in our Helm values.

### 3. Security Updates
- `golang.org/x/crypto` bump (#5233, #5308)
- `package.json` security updates (#5242)

**Action:** Accept these security updates.

### 4. Chromium Removal
Upstream removed chromium packaging:
```
- tools/package-chrome.sh (deleted in upstream)
```
**Our Status:** We still have this file. Should investigate if needed.

---

## Recommendations

### Option 1: Merge Strategy (Recommended)
Perform a merge with manual conflict resolution:

```bash
# 1. Create a sync branch
git checkout -b sync/upstream-dec-2025

# 2. Merge upstream
git merge upstream/main

# 3. Resolve conflicts
# - .gitignore: keep both sets of entries
# - logger/go.mod: keep our go 1.25.5 (upstream's 1.23.5 is inconsistent with their other modules)

# 4. Regenerate files
make generate  # regenerate deepcopy and openapi

# 5. Test build
make all
cd tools/entra-auth-provider && make build
cd ../keycloak-auth-provider && make build

# 6. Test Docker build
docker build -t obot-entraid:test .
```

### Option 2: Rebase Strategy (Not Recommended)
Would require replaying our 20 commits on top of upstream. More complex and risks losing commit history context.

### Option 3: Cherry-Pick Strategy (Alternative)
If merge produces unexpected issues, cherry-pick specific upstream commits:
- Security updates first (#5233, #5308, #5242)
- Bug fixes next
- UI overhaul last (most complex)

---

## Post-Merge Validation Checklist

- [ ] `.gitignore` has all our custom entries
- [ ] `logger/go.mod` has correct Go version (1.25.5 - keep ours)
- [ ] `apiclient/types/apppreferences.go` has `BrandingPreferences`
- [ ] `pkg/storage/apis/.../apppreferences.go` has `Branding` field
- [ ] `ui/user/src/routes/admin/app-preferences/+page.svelte` has Footer Branding section
- [ ] `Dockerfile` builds custom auth providers
- [ ] `chart/values.yaml` has our custom image and config
- [ ] `tools/entra-auth-provider/` exists and builds
- [ ] `tools/keycloak-auth-provider/` exists and builds
- [ ] `make all` succeeds
- [ ] `make lint` passes
- [ ] Docker image builds successfully
- [ ] Helm chart lints successfully

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| UI route changes break navigation | Low | Medium | Test all admin routes post-merge |
| Branding UI doesn't integrate | Low | Low | Manual merge of +page.svelte |
| Auth providers don't build | Very Low | High | Dockerfile has isolated build stages |
| Generated files out of sync | Medium | Low | Run `make generate` post-merge |
| Helm chart incompatible | Low | Medium | Test with `helm lint` and dry-run |

---

## Files Changed Summary

| Category | Count |
|----------|-------|
| Total files different | 258 |
| Actual conflicts | 2 |
| Files only in upstream | ~180 |
| Files only in our fork | ~30 |
| Files modified in both | ~15 |

---

## Next Steps - Detailed Merge Procedure

### Phase 1: Preparation
1. **Review this document** - understand all changes before starting
2. **Create backup branch:**
    ```bash
    git checkout main
    git checkout -b backup/pre-upstream-sync-dec-2025
    git push origin backup/pre-upstream-sync-dec-2025
    ```

   ### Phase 2: Initial Merge
3. **Create sync branch and merge:**
   ```bash
   git checkout main
   git checkout -b sync/upstream-dec-2025
   git merge upstream/main
   # This will report 2 conflicts
   ```

   ### Phase 3: Resolve Git Conflicts (2 files)
4. **Fix `.gitignore`:**
   - Keep ALL entries from both sides
   - Our entries: `.serena/`, `.archive/`, `.cloned-obot-tools/`, `*.tgz`
   - Upstream entry: `thoughts`

5. **Fix `logger/go.mod`:**
   - Keep our `go 1.25.5` (upstream's 1.23.5 is inconsistent)

6. **Stage conflict resolutions:**
   ```bash
   git add .gitignore logger/go.mod
   ```

   ### Phase 4: Manual File Merges (9 files)
7. **For each file in the Detailed Merge Guide above:**

   | File | Action |
   |------|--------|
   | `app-preferences/+page.svelte` | Accept upstream's `untrack`, re-add our branding UI |
   | `appPreferences.svelte.ts` | Keep upstream base, add our branding logic |
   | `chart/values.yaml` | Keep ours, update nanobot to v0.0.45 |
   | `Dockerfile` | Keep ours entirely |
   | `docker-build-and-push.yml` | Keep ours entirely |
   | `auth-providers/+page.svelte` | Accept upstream (except hardcoded "Google" bug) |
   | `apiclient/types/apppreferences.go` | Keep ours (additive) |
   | `pkg/storage/.../apppreferences.go` | Keep ours (additive) |

   ### Phase 5: Regenerate & Verify
8. **Regenerate generated files:**
   ```bash
   make generate
   ```

9. **Verify our types are included:**
   ```bash
   grep -n "BrandingPreferences" apiclient/types/zz_generated.deepcopy.go
   grep -n "BrandingPreferences" pkg/storage/openapi/generated/openapi_generated.go
   ```

   ### Phase 6: Build & Test
10. **Build everything:**
    ```bash
    make all
    make lint
    cd tools/entra-auth-provider && make build
    cd ../keycloak-auth-provider && make build
    ```

11. **Test Docker build:**
    ```bash
    docker build -t obot-entraid:sync-test .
    ```

12. **Test Helm chart:**
    ```bash
    helm lint chart/
    ```

    ### Phase 7: Commit & PR
13. **Commit the merge:**
    ```bash
    git add -A
    git commit -m "chore: merge upstream obot-platform/obot main (Dec 2025)

    Incorporates upstream changes including:
    - UI/UX overhaul (PR #5180)
    - Nanobot bump to v0.0.45
    - Security updates (crypto library)
    - Various bug fixes

    Preserves fork-specific features:
    - EntraID and Keycloak auth providers
    - BrandingPreferences feature
    - Custom Dockerfile and CI/CD"
    ```

14. **Push and create PR:**
    ```bash
    git push origin sync/upstream-dec-2025
    # Create PR on GitHub for review
    ```

    ### Phase 8: Post-Merge Validation
15. **Run through validation checklist** (see Post-Merge Validation Checklist section)

---

## Sources

- [obot-platform/obot Releases](https://github.com/obot-platform/obot/releases)
- [Introducing MCP Registry Support in Obot v0.14](https://obot.ai/blog/introducing-mcp-registry-support-in-obot-v0-14/)
- Upstream PR #5180: UI/UX Overhaul
- Upstream PR #5325: Nanobot OpenID configuration fix
