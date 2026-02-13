# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Obot is an open-source platform for implementing Model Context Protocol (MCP) technologies. It provides MCP hosting (Docker/Kubernetes), an MCP registry, an MCP gateway, and Obot Chat (a built-in chat client supporting OpenAI and Anthropic models).

## Tech Stack

- **Backend**: Go 1.26.0 with PostgreSQL (pgx), MCP protocol (`github.com/modelcontextprotocol/go-sdk`), gptscript, Kubernetes client libraries
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
