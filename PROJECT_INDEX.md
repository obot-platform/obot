# Project Index: Obot

Generated: 2025-12-07

## Project Summary

Obot is an open-source MCP (Model Context Protocol) Gateway and AI platform for cloud or on-premise deployment. It provides server discovery, OAuth 2.1 authentication, chat interfaces, and comprehensive admin tools.

## Project Structure

```
obot-entraid/
├── main.go              # Application entry point
├── generate.go          # Code generation directives
├── Makefile             # Build and development commands
├── Dockerfile           # Multi-stage Docker build
├── go.mod               # Go 1.25.3 module definition
│
├── pkg/                 # Core application code (414 Go files)
│   ├── cli/             # CLI commands (server, agents, invoke, etc.)
│   ├── api/             # API handlers (authn/, authz/, handlers/)
│   ├── controller/      # Kubernetes-style controllers
│   ├── server/          # HTTP server implementation
│   ├── storage/         # Database operations (GORM/PostgreSQL)
│   ├── gateway/         # MCP gateway logic
│   ├── mcp/             # MCP protocol implementation
│   ├── oauth/           # OAuth 2.1 authentication
│   ├── invoke/          # Tool invocation
│   └── ...              # (37 total packages)
│
├── apiclient/           # API client library
│   ├── types/           # API types (56 files)
│   └── *.go             # Client methods
│
├── ui/                  # Frontend applications
│   └── user/            # SvelteKit user interface
│
├── logger/              # Logging module (separate Go module)
├── tools/               # Development scripts
├── chart/               # Helm chart for Kubernetes
├── docs/                # Docusaurus documentation
└── tests/integration/   # Integration tests
```

## Entry Points

- **CLI Entry**: `main.go` -> `pkg/cli/root.go` (Obot struct)
- **Server**: `pkg/cli/server.go` (Server.Run)
- **User UI**: `ui/user/` (SvelteKit application)
- **Tests**: `tests/integration/integration_test.go`

## Core Modules

### pkg/cli - Command Line Interface
- Root command and subcommands (server, agents, invoke, threads, etc.)
- Uses Cobra framework with struct tags for flags

### pkg/api - HTTP API
- `authn/` - Authentication middleware
- `authz/` - Authorization logic
- `handlers/` - REST API endpoints

### pkg/controller - Background Controllers
- Kubernetes-style controllers for resource management

### pkg/storage - Data Layer
- GORM-based PostgreSQL storage
- pgvector for vector search capabilities

### apiclient/types - API Types
- 56 type definition files
- Kubernetes-style types with DeepCopy methods
- Generated code in `zz_generated.deepcopy.go`

## Configuration

| File | Purpose |
|------|---------|
| `.golangci.yaml` | Go linter config (errcheck, govet, revive, etc.) |
| `.goreleaser.yaml` | Release automation |
| `workflow.yaml` | CI/CD workflow |
| `chart/values.yaml` | Helm deployment values |

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/gptscript-ai/gptscript` | AI scripting engine |
| `github.com/modelcontextprotocol/go-sdk` | MCP protocol |
| `gorm.io/gorm` | Database ORM |
| `k8s.io/api`, `k8s.io/apimachinery` | Kubernetes API types |
| `github.com/spf13/cobra` | CLI framework |

## Quick Start

### Development
```bash
# Run dev environment (API + UIs)
make dev

# Build binary
make build

# Run tests
make test

# Lint code
make lint
```

### Docker
```bash
# Build image
docker build -t my-obot .

# Run container
docker run -p 8080:8080 my-obot
```

### User UI Development
```bash
cd ui/user
pnpm install
pnpm run dev
```

## Test Coverage

- Unit tests: 14 files in `pkg/`
- Integration tests: `tests/integration/`
- Run with: `make test` (unit) or `make test-integration`

## Environment Variables

Run `./bin/obot server --help` after building to see all configuration options. Key ones:
- `OPENAI_API_KEY` - OpenAI API key
- `ANTHROPIC_API_KEY` - Anthropic API key
- `GPTSCRIPT_TOOL_REMAP` - Local tool development
