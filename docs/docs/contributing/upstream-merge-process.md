# Upstream Merge Process

This document describes the process for safely merging changes from the upstream `obot-platform/obot` repository into our fork `jrmatherly/obot-entraid`.

## Overview

Our fork adds custom authentication providers (Entra ID, Keycloak) and related infrastructure not available in upstream. When merging upstream changes, we must verify our customizations remain intact.

## Prerequisites

Ensure the upstream remote is configured:

```bash
# Check remotes
git remote -v

# If upstream not configured, add it
git remote add upstream https://github.com/obot-platform/obot.git
```

## Step-by-Step Verification Process

### Step 1: Fetch Latest Upstream

```bash
git fetch upstream
```

### Step 2: Analyze Divergence

```bash
# Count commits ahead/behind
git rev-list --count main...upstream/main --left-only   # Your commits ahead
git rev-list --count main...upstream/main --right-only  # Upstream commits behind

# Find common ancestor (merge base)
git merge-base main upstream/main
```

### Step 3: Review Commit History

```bash
# Store merge base for reuse
MERGE_BASE=$(git merge-base main upstream/main)

# View your fork's commits since divergence
git log --oneline ${MERGE_BASE}..main

# View upstream commits since divergence
git log --oneline ${MERGE_BASE}..upstream/main
```

### Step 4: Identify Overlapping Files (Conflict Risk)

```bash
MERGE_BASE=$(git merge-base main upstream/main)

# Files changed in your fork
git diff --name-only ${MERGE_BASE}..main | sort > /tmp/fork_files.txt

# Files changed in upstream
git diff --name-only ${MERGE_BASE}..upstream/main | sort > /tmp/upstream_files.txt

# Files modified in BOTH (potential conflicts)
comm -12 /tmp/fork_files.txt /tmp/upstream_files.txt
```

### Step 5: Dry-Run Merge Test

This is the critical step - test the merge without committing:

```bash
# Perform merge without committing
git merge --no-commit --no-ff upstream/main

# Check for conflicts
git status

# If conflicts exist, they'll be listed as "both modified"
# If clean, you'll see "All conflicts fixed but you are still merging"
```

### Step 6: Verify Custom Changes Preserved

After the dry-run merge, verify your customizations are intact:

```bash
# Check custom auth providers exist
ls -la tools/entra-auth-provider/
ls -la tools/keycloak-auth-provider/

# Check custom tool registry
cat tools/index.yaml

# Check Dockerfile still merges registries
grep -A5 "Merge index.yaml" Dockerfile

# Check GitHub workflows
ls -la .github/workflows/docker-build-and-push.yml
ls -la .github/workflows/helm.yml
```

### Step 7: Review Merged Files

For files that were modified in both forks, inspect the merge result:

```bash
# View what changed in a specific file
git diff HEAD <filename>

# Example for common conflict files:
git diff HEAD chart/values.yaml
git diff HEAD pkg/proxy/proxy.go
git diff HEAD go.mod
```

### Step 8: Abort or Commit

Based on your findings:

```bash
# If issues found - abort and investigate
git merge --abort

# If everything looks good - commit the merge
git commit -m "chore: merge upstream obot-platform/obot main"
```

## Our Custom Changes to Verify

When merging upstream, ensure these customizations remain:

### Custom Auth Providers

| Path | Description |
|------|-------------|
| `tools/entra-auth-provider/` | Microsoft Entra ID authentication |
| `tools/keycloak-auth-provider/` | Keycloak OIDC authentication |
| `tools/auth-providers-common/` | Shared auth provider utilities |
| `tools/placeholder-credential/` | Credential placeholder tool |
| `tools/index.yaml` | Custom tool registry |

### Build Infrastructure

| Path | Description |
|------|-------------|
| `Dockerfile` | Must merge upstream + custom tools into unified registry |
| `.github/workflows/docker-build-and-push.yml` | GHCR container publishing |
| `.github/workflows/helm.yml` | GHCR Helm chart publishing |

### Helm Chart Customizations

| Path | Description |
|------|-------------|
| `chart/Chart.yaml` | Our version numbering |
| `chart/values.yaml` | Custom defaults and MCP configuration |
| `chart/templates/deployment.yaml` | Health probe customizations |

### Documentation

| Path | Description |
|------|-------------|
| `docs/ENTRA_ID_IMPLEMENTATION_SPEC.md` | Entra ID implementation details |
| `tools/README.md` | Auth provider documentation |
| `tools/keycloak-auth-provider/KEYCLOAK_SETUP.md` | Keycloak setup guide |

### UI Customizations

| Path | Description |
|------|-------------|
| `ui/user/src/lib/components/navbar/Profile.svelte` | Profile picture handling |
| `ui/user/src/routes/admin/auth-providers/+page.svelte` | Auth provider UI |
| `ui/user/src/routes/terms-of-service/+page.svelte` | Terms of Service with fork GitHub URL |
| `ui/user/src/routes/privacy-policy/+page.svelte` | Privacy Policy with fork GitHub URL |

## Quick Reference Commands

```bash
# Full verification in one sequence
git fetch upstream && \
MERGE_BASE=$(git merge-base main upstream/main) && \
echo "=== Commits ahead: $(git rev-list --count main...upstream/main --left-only)" && \
echo "=== Commits behind: $(git rev-list --count main...upstream/main --right-only)" && \
echo "=== Files modified in both:" && \
comm -12 \
  <(git diff --name-only ${MERGE_BASE}..main | sort) \
  <(git diff --name-only ${MERGE_BASE}..upstream/main | sort)

# Test merge
git merge --no-commit --no-ff upstream/main

# Verify and either abort or commit
git merge --abort  # OR
git commit -m "chore: merge upstream obot-platform/obot main"
```

## Troubleshooting

### Merge Conflicts

If conflicts occur:

1. Identify conflicted files: `git status | grep "both modified"`
2. Open each file and resolve conflicts (look for `<<<<<<<`, `=======`, `>>>>>>>` markers)
3. After resolving: `git add <resolved-file>`
4. Complete merge: `git commit`

### Common Conflict Areas

| File | Our Changes | Upstream Changes | Resolution Strategy |
|------|-------------|------------------|---------------------|
| `chart/values.yaml` | MCP config, tool registry | Version bumps | Keep both, ensure our config preserved |
| `go.mod` | Go version, deps | Upstream deps | Usually auto-merges; verify Go version |
| `pkg/proxy/proxy.go` | Auth provider integration | Refactoring | Careful review required |

### Rollback

If merge causes issues after commit:

```bash
# Find the commit before merge
git log --oneline -5

# Reset to pre-merge state
git reset --hard <commit-before-merge>

# Or create a revert commit
git revert -m 1 HEAD
```

## Post-Merge Verification

After completing the merge:

```bash
# Build and test
make build
make test

# Lint
make lint

# Verify Docker build
docker build -t obot-entraid:test .

# Check tool registry in container
docker run --rm obot-entraid:test ls -la /obot-tools/tools/
docker run --rm obot-entraid:test cat /obot-tools/tools/index.yaml
```

## Automation Considerations

For frequent upstream syncs, consider:

1. **Scheduled checks**: GitHub Actions workflow to check for upstream updates weekly
2. **Automated dry-run**: CI job that tests merge compatibility
3. **Notification**: Alert when new upstream releases are available
4. **Changelog comparison**: Automated diff of release notes
