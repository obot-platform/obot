# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Obot is an open-source platform for implementing Model Context Protocol (MCP) technologies. It provides MCP hosting (Docker/Kubernetes), an MCP registry, an MCP gateway, and Obot Chat (a built-in chat client supporting OpenAI and Anthropic models).

## Tech Stack

- **Backend**: Go 1.26.0 with PostgreSQL (pgx), MCP protocol (`github.com/modelcontextprotocol/go-sdk`), Kubernetes client libraries
- **Frontend**: SvelteKit 5 with Vite, Tailwind CSS 4, TypeScript, CodeMirror 6, Milkdown (markdown editor)
- **Documentation**: Docusaurus 3 (in `/docs`)

## Common Commands

### Development
```bash
make dev              # Run full dev environment (Go server + SvelteKit UI) with hot reload
make dev-open         # Same as above, but opens browser automatically
```

### Building
```bash
make build            # Build Go binary to bin/obot
make ui               # Build user UI (both browser and Node targets)
make all              # Build UI + Go binary
```

### Testing
```bash
make test             # Run all Go tests (excludes integration tests)
make test-integration # Run integration tests
```

### Linting & Formatting
```bash
make lint             # Run Go linters (golangci-lint)
make tidy             # Tidy Go modules
make validate-go-code # Run tidy, generate, lint, and check for uncommitted changes
```

### UI Development (in ui/user/)
```bash
pnpm install          # Install dependencies
pnpm run dev          # Start dev server
pnpm run check        # TypeScript type checking
pnpm run lint         # ESLint + Prettier check
pnpm run format       # Auto-format code
pnpm run ci           # Run format, lint, and check
```

### Documentation (in docs/)
```bash
make serve-docs       # Start local docs server
```

## Architecture

### Entry Points

- `main.go` - Application entry, delegates to CLI
- `pkg/cli/server.go` - Server command, initializes services and starts HTTP server
- `pkg/server/server.go` - HTTP server setup, CORS, middleware

### Directory Structure

- `/pkg` - Core Go packages
  - `api/` - REST API implementation with handlers in `api/handlers/`
  - `controller/` - Kubernetes-style controllers and data handlers
  - `mcp/` - MCP protocol implementation (Docker and Kubernetes runners)
  - `storage/` - CRD-style storage layer with resource types in `apis/obot.obot.ai/v1/`
  - `gateway/` - MCP gateway for proxying and access control
  - `invoke/` - Agent/workflow invocation engine (integrates with GPTScript)
  - `services/` - Dependency injection container (`config.go` has all service dependencies)
  - `cli/` - CLI command implementations
  - `auth/`, `oauth/`, `jwt/` - Authentication/authorization
- `/ui/user` - SvelteKit user-facing application
  - `src/lib/components/` - Reusable Svelte components organized by feature
  - `src/lib/services/` - HTTP client and API interaction logic
  - `src/routes/` - SvelteKit file-based routing
- `/apiclient` - Go module for API client code
- `/logger` - Go module for logging utilities
- `/tools` - Development scripts (`dev.sh`, `devmode-kubeconfig`)
- `/chart` - Helm chart for Kubernetes deployment
- `/tests/integration` - Integration tests

### MCP Server Types and Runtimes

**Server Types:**
- **Single-user**: No multitenancy - Obot deploys an instance per user. Stored as `MCPServerCatalogEntry` with runtime `npx`, `uvx`, or `containerized`
- **Multi-user**: Supports multitenancy - one instance for all users. Stored as `MCPServer`
- **Remote**: Runs outside Obot. Stored as `MCPServerCatalogEntry` with runtime `remote`
- **Composite**: Points to tools from multiple other servers. Stored as `MCPServerCatalogEntry` with runtime `composite`

**Runtimes:**
- `npx`: NPM package (STDIO transport only)
- `uvx`: PyPI package (STDIO transport only)
- `containerized`: Docker container image (HTTP transport)
- `remote`: Hosted MCP server elsewhere (HTTP transport)
- `composite`: Pointer to tools from multiple servers

**Key Concepts:**
- `MCPServerCatalogEntry` - Server template/configuration that can be instantiated
- `MCPServer` - Fully configured and running server
- `MCPServerInstance` - Individual user's connection to a multi-user server (for auditing)
- All admin-configured servers belong to the `default` MCPCatalog

### MCP Registry API

Obot serves the MCP Registry API (open standard) at `/v0.1` routes.

### Obot Chat

Users create Projects (configurations of MCP servers) and can add any MCPServers/MCPServerCatalogEntries they have access to. Each project supports multiple chat threads.

### Power User Workspaces

Users with Power User role (or higher) have their own PowerUserWorkspace for creating/managing personal MCP servers. Power User Plus can also grant access to others via AccessControlRules.

### API Structure

REST API handlers are in `/pkg/api/handlers/`. Each handler file corresponds to a resource type (agents, assistants, threads, credentials, etc.). The API server runs on port 8080 by default.

## Go Linting Configuration

Uses golangci-lint v2.9.0 with these linters enabled: errcheck, govet, ineffassign, revive, staticcheck, thelper, unused, whitespace. Formatters: gofmt, goimports.

## Module Structure

Main module with local sub-modules:
- `github.com/obot-platform/obot` (main)
- `github.com/obot-platform/obot/apiclient` → `./apiclient`
- `github.com/obot-platform/obot/logger` → `./logger`

## CI Gates and Release Contract (Accelerate fork)

This fork publishes `ghcr.io/accelerate-data/obot-vibedata` (`:latest` and `:<upstream-version>-vibedata`). Studio's nightly release pipeline resolves the `:latest` digest at candidate time and pins it into the candidate tag (vd-studio `docs/functional/release-management/README.md`). Treat `main` and the publish workflow as release inputs.

### Required checks on `main`

The GitHub ruleset is versioned at `.github/rulesets/main-branch.json`. Create it with `gh api repos/accelerate-data/obot/rulesets --input .github/rulesets/main-branch.json`; update an existing one with `--method PUT` against `.../rulesets/<id>`. Required contexts:

- `lint-go` (`go.yml`) — `make build` + `make validate-go-code`. Runs on every PR; do not add paths filters to a required check or non-matching PRs can never merge.
- `test` (`test.yaml`) — `make test`.
- `verify` (`verify-sync-metadata.yml`) — guards `.accelerate/upstream-sync.json`, which drives the published image version tag.

Deliberately not required: `user` UI lint (paths-filtered to `ui/user/**`) and helm `lint` (chart-only).

Upstream-sync PRs are drafts created with `GITHUB_TOKEN`, whose events trigger no workflows — the required checks start when a maintainer marks the PR ready for review. If the branch is updated by automation after that, close and reopen the PR to re-trigger the checks.

### Image publication is fail-closed

`build-vibedata-image.yml` (push to `main` or manual dispatch) is the only workflow that advances Studio-consumable tags; upstream's `docker-build-and-push.yml` has publishing disabled. Stage order: resolve version from sync metadata → verify both provider images exist and capture digests → per-arch builds pushed by digest only (untagged) → manifest merge creates the `:latest`/versioned tags → both-arch verification → cosign signing. A failure at any stage leaves the previous `:latest` untouched. Provider inputs are pinned to the pre-verified digests, so the image checked is the image consumed.

If Studio picks up a broken or stale obot input, the owning failure is a red `Build and Push obot-vibedata` run on `main` in this repo.

### Local verification

- `make build && make validate-go-code` (installs the pinned golangci-lint if missing)
- `make test` (or a narrower `go test ./pkg/...`)
- `bash scripts/verify-sync-metadata.sh` after touching sync metadata
