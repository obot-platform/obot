# Custom Tools Directory

This directory contains custom authentication providers and supporting files for the obot-entraid project.

## Directory Structure

```
tools/
├── auth-providers-common/     # Shared utilities for auth providers
├── entra-auth-provider/       # Microsoft Entra ID (Azure AD) auth provider
├── keycloak-auth-provider/    # Keycloak OIDC auth provider
├── placeholder-credential/    # Fake credential tool (required by auth providers)
├── combine-envrc.sh           # Script to merge .envrc files in container builds
├── dev.sh                     # Development server launcher
├── devmode-kubeconfig         # Kubeconfig for local development
├── index.yaml                 # Custom tool registry definition
├── package-chrome.sh          # Chrome packaging script for container builds
├── tool.gpt                   # GPTScript wrapper for loading index.yaml
└── vendor.go                  # Go import dependencies for code generation
```

## Local Development

### Prerequisites

1. Go 1.25.3 or later
2. Node.js and pnpm
3. Docker (for container builds)

### Running Locally

From the project root:

```bash
# Run in development mode (uses ./tools as local registry)
make dev

# Or with browser tabs auto-opening
make dev-open
```

The `.envrc.dev` file configures local development:
```bash
export OBOT_SERVER_TOOL_REGISTRIES=github.com/obot-platform/tools,./tools
```

This means:
- Upstream tools are fetched from `github.com/obot-platform/tools`
- Custom auth providers are loaded from `./tools` (this directory)

### Building Auth Providers

Each auth provider can be built independently:

```bash
# Build EntraID provider
cd tools/entra-auth-provider
make build

# Build Keycloak provider
cd tools/keycloak-auth-provider
make build
```

Or build everything via Docker:

```bash
docker build -t obot-test .
```

## Auth Provider Configuration

### Microsoft Entra ID

Required environment variables:
- `OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID` - Azure App Registration client ID
- `OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET` - Azure App Registration client secret
- `OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID` - Azure tenant ID (or 'common'/'organizations')
- `OBOT_AUTH_PROVIDER_COOKIE_SECRET` - Base64-encoded 16/24/32 byte secret
- `OBOT_AUTH_PROVIDER_EMAIL_DOMAINS` - Allowed email domains (use `*` for all)

Optional:
- `OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS` - Comma-separated Azure AD group IDs
- `OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS` - Required if tenant is 'common'/'organizations'
- `OBOT_ENTRA_AUTH_PROVIDER_GROUP_CACHE_TTL` - Group cache duration (default: `1h`)
- `OBOT_ENTRA_AUTH_PROVIDER_ICON_CACHE_TTL` - Profile picture cache duration (default: `24h`)
- `OBOT_AUTH_INSECURE_COOKIES` - Set to `true` for local development with HTTP (default: `false`, **NEVER enable in production**)
- `OBOT_AUTH_PROVIDER_COOKIE_DOMAIN` - Override cookie domain (default: hostname from server URL)
- `OBOT_AUTH_PROVIDER_COOKIE_PATH` - Override cookie path (default: `/`)
- `OBOT_AUTH_PROVIDER_COOKIE_SAMESITE` - Override SameSite attribute (default: `Lax`, options: `Strict`, `Lax`, `None`)

See `entra-auth-provider/README.md` for detailed Azure setup instructions.

### Keycloak

Required environment variables:
- `OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID` - Keycloak client ID
- `OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET` - Keycloak client secret
- `OBOT_KEYCLOAK_AUTH_PROVIDER_URL` - Keycloak base URL
- `OBOT_KEYCLOAK_AUTH_PROVIDER_REALM` - Keycloak realm name
- `OBOT_AUTH_PROVIDER_COOKIE_SECRET` - Base64-encoded 16/24/32 byte secret
- `OBOT_AUTH_PROVIDER_EMAIL_DOMAINS` - Allowed email domains (use `*` for all)

Optional:
- `OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS` - Comma-separated group names
- `OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES` - Comma-separated role names
- `OBOT_KEYCLOAK_AUTH_PROVIDER_GROUP_CACHE_TTL` - Group cache duration (default: `1h`)
- `OBOT_AUTH_INSECURE_COOKIES` - Set to `true` for local development with HTTP (default: `false`, **NEVER enable in production**)
- `OBOT_AUTH_PROVIDER_COOKIE_DOMAIN` - Override cookie domain (default: hostname from server URL)
- `OBOT_AUTH_PROVIDER_COOKIE_PATH` - Override cookie path (default: `/`)
- `OBOT_AUTH_PROVIDER_COOKIE_SAMESITE` - Override SameSite attribute (default: `Lax`, options: `Strict`, `Lax`, `None`)

## Container Build Architecture

The Dockerfile uses a **patched tools** approach:

1. **Upstream tools** are pulled from `ghcr.io/obot-platform/tools:latest`
2. **Custom auth providers** (EntraID, Keycloak) are built and copied into `/obot-tools/tools/`
3. **index.yaml is merged** using `yq` to combine upstream and custom auth providers
4. **Single unified registry** at `/obot-tools/tools` contains all providers

This eliminates the complexity of multiple tool registries and ensures all auth providers (GitHub, Google, EntraID, Keycloak) are available from a single location.

### Verifying the Container Build

```bash
# Build the image
docker build -t obot-test .

# Verify auth providers are merged
docker run --rm --entrypoint sh obot-test -c 'grep -A 10 "authProviders:" /obot-tools/tools/index.yaml'

# Expected output shows all 4 providers:
# authProviders:
#   github-auth-provider:
#     reference: ./github-auth-provider
#   google-auth-provider:
#     reference: ./google-auth-provider
#   entra-auth-provider:
#     reference: ./entra-auth-provider
#   keycloak-auth-provider:
#     reference: ./keycloak-auth-provider

# Verify tool registry path
docker run --rm --entrypoint sh obot-test -c 'cat /obot-tools/.envrc.tools | grep TOOL_REGISTRIES'
# Expected: export OBOT_SERVER_TOOL_REGISTRIES="/obot-tools/tools"
```

## Modifying Auth Providers

### Adding a New Auth Provider

1. Create a new directory: `tools/my-auth-provider/`
2. Add required files:
   - `tool.gpt` - GPTScript tool definition with metadata
   - `main.go` - OAuth2 proxy implementation
   - `Makefile` - Build targets
   - `go.mod` / `go.sum` - Go module dependencies
3. Update `tools/index.yaml` to include the new provider
4. Update `Dockerfile` to build and copy the new provider

### Local Development with Modified auth-providers-common

If you need to modify `auth-providers-common` and test locally, add replace directives to the auth provider's `go.mod`:

```go
// In entra-auth-provider/go.mod or keycloak-auth-provider/go.mod
replace github.com/obot-platform/tools/auth-providers-common => ../auth-providers-common
```

Then rebuild:
```bash
cd tools/entra-auth-provider
go mod tidy
make build
```

## Troubleshooting

### Auth provider not appearing in UI

1. Check the tool registry is correctly configured:
   ```bash
   # In container
   cat /obot-tools/.envrc.tools | grep TOOL_REGISTRIES
   ```

2. Verify index.yaml contains the auth provider:
   ```bash
   cat /obot-tools/tools/index.yaml | grep -A 2 "entra-auth-provider"
   ```

3. Check the provider binary exists:
   ```bash
   ls -la /obot-tools/tools/entra-auth-provider/bin/
   ```

### OAuth errors

1. Verify environment variables are set correctly
2. Check Azure/Keycloak redirect URI matches `{OBOT_SERVER_URL}/oauth2/callback`
3. Review provider logs for detailed error messages

### Build failures

1. Ensure Go 1.25.3+ is installed
2. Run `go mod tidy` in the auth provider directory
3. Check for missing dependencies in `go.sum`
