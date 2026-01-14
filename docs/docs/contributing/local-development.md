---
sidebar_position: 3
title: Local Development
description: Running Obot locally with custom authentication providers and tool registries
---

# Local Development

This guide explains how to run Obot locally for development, including working with custom authentication providers and the local tool registry.

## Prerequisites

Before starting local development, ensure you have:

1. **Go 1.25.3 or later** - Required for building auth providers
2. **Node.js and pnpm** - Required for UI development
3. **Docker** - Required for container builds and dependencies
4. **Git** - For version control

## Quick Start

The fastest way to start Obot in development mode:

```bash
# Run in development mode (uses ./tools as local registry)
make dev

# Or with browser tabs auto-opening
make dev-open
```

This command:
- Starts the Obot server with hot reload
- Serves the UI in development mode
- Configures local tool registry (from `./tools` directory)
- Opens browser tabs for UI and admin interface (with `dev-open`)

---

## Tool Registry Configuration

Obot uses tool registries to discover authentication providers and MCP servers. In development mode, the local tool registry is enabled via `.envrc.dev`:

```bash
export OBOT_SERVER_TOOL_REGISTRIES=github.com/obot-platform/tools,./tools
```

This configuration means:
- **Upstream tools**: Fetched from `github.com/obot-platform/tools`
- **Custom tools**: Loaded from `./tools` (local directory)

The `./tools` directory contains:
- `entra-auth-provider/` - Microsoft Entra ID authentication
- `keycloak-auth-provider/` - Keycloak authentication
- `auth-providers-common/` - Shared auth provider utilities
- `placeholder-credential/` - Test credential provider
- `index.yaml` - Local tool registry definition

---

## Building Authentication Providers

### Individual Provider Builds

Build auth providers independently during development:

```bash
# Build Entra ID provider
cd tools/entra-auth-provider
make build

# Build Keycloak provider
cd tools/keycloak-auth-provider
make build
```

### Full Docker Build

Build the complete Obot image with custom auth providers:

```bash
docker build -t obot-local .
```

The Docker build uses a **patched tools** approach:
1. Pulls upstream tools from `ghcr.io/obot-platform/tools:latest`
2. Builds custom auth providers (Entra ID, Keycloak)
3. Merges `index.yaml` to combine upstream and custom providers
4. Creates a unified registry at `/obot-tools/tools`

### Verifying the Container Build

```bash
# Build the image
docker build -t obot-local .

# Verify auth providers are merged
docker run --rm --entrypoint sh obot-local -c \
  'grep -A 10 "authProviders:" /obot-tools/tools/index.yaml'

# Expected output shows all providers:
# authProviders:
#   github-auth-provider:
#     reference: ./github-auth-provider
#   google-auth-provider:
#     reference: ./google-auth-provider
#   entra-auth-provider:
#     reference: ./entra-auth-provider
#   keycloak-auth-provider:
#     reference: ./keycloak-auth-provider
```

---

## Environment Variables for Development

### Common Development Variables

```bash
# Server configuration
export OBOT_SERVER_PUBLIC_URL="http://localhost:8080"

# Enable insecure cookies for HTTP (NEVER in production!)
export OBOT_AUTH_INSECURE_COOKIES="true"

# Cookie secret for session encryption
export OBOT_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)

# Email domain restriction
export OBOT_AUTH_PROVIDER_EMAIL_DOMAINS="*"  # Allow all domains
```

### Entra ID Development Configuration

```bash
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID="your-dev-client-id"
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET="your-dev-client-secret"
export OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID="your-tenant-id"

# Optional: restrict to specific groups
export OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS="group-id-1,group-id-2"

# Optional: cache tuning
export OBOT_ENTRA_AUTH_PROVIDER_GROUP_CACHE_TTL="1h"
export OBOT_ENTRA_AUTH_PROVIDER_ICON_CACHE_TTL="24h"
```

See [Entra ID Authentication Setup](../configuration/entra-id-authentication.md) for Azure App Registration configuration.

### Keycloak Development Configuration

```bash
export OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID="obot"
export OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET="your-client-secret"
export OBOT_KEYCLOAK_AUTH_PROVIDER_URL="http://localhost:8180"
export OBOT_KEYCLOAK_AUTH_PROVIDER_REALM="obot"

# Optional: restrict to specific groups/roles
export OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS="admin,developers"
export OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES="obot-user"

# Optional: cache tuning
export OBOT_KEYCLOAK_AUTH_PROVIDER_GROUP_CACHE_TTL="1h"
```

See [Keycloak Authentication Setup](../configuration/keycloak-authentication.md) for Keycloak client configuration.

---

## Modifying Auth Providers

### Working with auth-providers-common

If you need to modify shared auth provider code:

1. Edit files in `tools/auth-providers-common/`
2. Add replace directive to auth provider's `go.mod`:

```go
// In entra-auth-provider/go.mod or keycloak-auth-provider/go.mod
replace github.com/obot-platform/tools/auth-providers-common => ../auth-providers-common
```

1. Rebuild the provider:

```bash
cd tools/entra-auth-provider
go mod tidy
make build
```

### Adding a New Auth Provider

To create a new authentication provider:

1. **Create Provider Directory**
   ```bash
   mkdir -p tools/my-auth-provider
   cd tools/my-auth-provider
   ```

2. **Create Required Files**
   - `tool.gpt` - GPTScript tool definition with metadata
   - `main.go` - OAuth2 proxy implementation
   - `Makefile` - Build targets
   - `go.mod` / `go.sum` - Go module dependencies

3. **Update Tool Registry**

   Add your provider to `tools/index.yaml`:
   ```yaml
   authProviders:
     my-auth-provider:
       reference: ./my-auth-provider
       description: My Custom Authentication Provider
   ```

4. **Update Dockerfile**

   Add build steps in the Dockerfile to compile and copy your provider.

5. **Test Locally**
   ```bash
   make dev
   # Your provider should appear in the authentication providers list
   ```

---

## Development Workflow

### Standard Development Loop

1. **Make code changes** to auth providers or server code
2. **Rebuild** affected components:
   ```bash
   # For auth provider changes
   cd tools/entra-auth-provider && make build

   # For server changes
   make build
   ```
3. **Restart development server**:
   ```bash
   make dev
   ```
4. **Test changes** in browser at http://localhost:8080
5. **Run tests**:
   ```bash
   make test
   ```

### Hot Reload

The UI supports hot reload automatically. Changes to TypeScript/Svelte files will be reflected immediately in the browser.

For server changes, you'll need to restart with `make dev`.

---

## Troubleshooting

### Auth Provider Not Appearing in UI

**Problem**: Custom auth provider doesn't show up in authentication providers list

**Solution**:
1. Verify tool registry configuration:
   ```bash
   cat .envrc.dev | grep TOOL_REGISTRIES
   # Should include: ./tools
   ```

2. Check `index.yaml` includes your provider:
   ```bash
   cat tools/index.yaml | grep -A 2 "my-auth-provider"
   ```

3. Ensure provider binary exists:
   ```bash
   ls -la tools/my-auth-provider/bin/
   ```

4. Check Obot server logs for tool registry errors

### OAuth Errors During Local Testing

**Problem**: OAuth flow fails with redirect URI mismatch

**Solution**:
1. Verify environment variable is set correctly:
   ```bash
   echo $OBOT_SERVER_PUBLIC_URL
   # Should match: http://localhost:8080
   ```

2. Ensure Azure/Keycloak redirect URI matches:
   ```
   http://localhost:8080/oauth2/callback
   ```

3. For local HTTP testing, ensure insecure cookies are enabled:
   ```bash
   export OBOT_AUTH_INSECURE_COOKIES="true"
   ```

### Build Failures

**Problem**: `make build` or `go build` fails

**Solution**:
1. Verify Go version:
   ```bash
   go version
   # Should be 1.25.3 or later
   ```

2. Clean and rebuild:
   ```bash
   go clean -cache
   go mod tidy
   make build
   ```

3. Check for missing dependencies:
   ```bash
   go mod verify
   ```

---

## See Also

- [Entra ID Authentication](../configuration/entra-id-authentication.md) - Azure setup guide
- [Keycloak Authentication](../configuration/keycloak-authentication.md) - Keycloak setup guide
- [Authentication Provider Testing](../operations/auth-provider-testing.md) - Testing procedures
- [Contributor Guide](./contributor-guide.md) - General contribution guidelines
- [Upstream Merge Process](./upstream-merge-process.md) - Syncing with upstream

---

*Last Updated: 2026-01-13*
*For detailed auth provider implementation, see: `tools/README.md` in the repository*