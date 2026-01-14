# Contributing to obot-entraid

Thank you for your interest in contributing to obot-entraid! This project is a fork of [obot-platform/obot](https://github.com/obot-platform/obot) with custom authentication providers for Microsoft Entra ID and Keycloak.

## Quick Links

- **[Full Contributor Guide](docs/docs/contributing/contributor-guide.md)** - Comprehensive development guide
- **[Fork Workflow Analysis](docs/docs/contributing/fork-workflow-analysis-2026.md)** - Upstream merge strategy
- **[Upstream Merge Process](docs/docs/contributing/upstream-merge-process.md)** - Step-by-step merge guide
- **[Communication Templates](docs/docs/contributing/communication-templates.md)** - Notification templates
- **[Pull Request Template](.github/PULL_REQUEST_TEMPLATE.md)** - PR checklist

## At a Glance

### Project Structure

```
obot-entraid/
├── tools/                          # Custom authentication providers (fork-specific)
│   ├── entra-auth-provider/       # Microsoft Entra ID (Azure AD)
│   └── keycloak-auth-provider/    # Keycloak OIDC
├── .github/workflows/              # Fork-specific CI/CD workflows
├── docs/docs/contributing/         # Contribution documentation
├── pkg/                            # Go backend (shared with upstream)
├── ui/user/                        # SvelteKit UI (shared with upstream)
└── chart/                          # Helm chart (fork customizations)
```

### Tech Stack

- **Backend**: Go 1.25.5
- **Frontend**: SvelteKit 5, TypeScript, Tailwind CSS 4
- **Database**: PostgreSQL
- **Deployment**: Docker, Kubernetes, Helm
- **Authentication**: OAuth2/OIDC (custom providers)

## Getting Started

### Prerequisites

- Go 1.25.5 or later
- Node.js 18+ and pnpm
- Docker (for testing)
- Git

### Quick Setup

```bash
# 1. Fork and clone
git clone https://github.com/YOUR_USERNAME/obot-entraid.git
cd obot-entraid

# 2. Add upstream remote
git remote add upstream https://github.com/jrmatherly/obot-entraid.git

# 3. Install dependencies
make build           # Build Go backend
cd ui/user && pnpm install && cd ../..  # Install UI dependencies

# 4. Run tests
make test            # Go tests
make lint            # Go linting
cd ui/user && pnpm run ci && cd ../..   # UI tests + linting
```

### Development Workflow

```bash
# 1. Create feature branch
git checkout main
git pull upstream main
git checkout -b feature/your-feature-name

# 2. Make changes and commit
# Follow Conventional Commits: feat|fix|docs|test|chore|refactor(scope): message
git commit -m "feat(entra): add MFA support"

# 3. Test locally
make build && make test && make lint

# 4. Push and create PR
git push origin feature/your-feature-name
# Use GitHub UI or `gh pr create`
```

## Contribution Areas

### High Priority

🔐 **Authentication Providers**
- LDAP/Active Directory integration
- SAML 2.0 support
- Okta, Auth0, generic OIDC providers

📚 **Documentation**
- Setup guides for different environments
- Troubleshooting documentation
- API documentation

🧪 **Testing**
- Integration tests for auth providers
- E2E tests for authentication flows

### Contributing Rules

#### What We Welcome

✅ Bug fixes for authentication providers
✅ New authentication provider implementations
✅ Documentation improvements
✅ Test coverage expansion
✅ Performance optimizations
✅ Security enhancements

#### What Requires Discussion First

⚠️ Breaking changes to existing auth providers
⚠️ Major architectural changes
⚠️ Changes to upstream-shared code (may conflict with future merges)

**Open a GitHub Discussion or Issue before starting work on these!**

## Code Standards

### Go Code

```go
// Follow Effective Go guidelines
// Document exported functions and types
package provider

// FetchUserProfile retrieves user profile from Microsoft Graph API.
// Returns user profile data or error if request fails.
func FetchUserProfile(ctx context.Context, token string) (map[string]interface{}, error) {
    // Implementation...
}
```

**Quality checks:**
```bash
gofmt -w .                    # Format code
golangci-lint run             # Lint code
go test ./...                 # Run tests
```

### TypeScript/Svelte

```typescript
// Use TypeScript for type safety
// Follow ESLint configuration
export interface UserProfile {
  id: string;
  name: string;
  email: string;
  iconUrl?: string;
}
```

**Quality checks:**
```bash
cd ui/user
pnpm run check               # TypeScript type checking
pnpm run lint                # ESLint
pnpm run format              # Prettier
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:** `feat`, `fix`, `docs`, `test`, `chore`, `refactor`, `perf`, `ci`

**Examples:**
```bash
feat(entra): add conditional access policy support
fix(keycloak): resolve token expiration handling
docs: update Entra ID setup guide with MFA configuration
test(auth): add integration tests for profile picture handling
```

## Fork-Specific Guidelines

### Custom Files (Never Merge from Upstream)

These files are fork-specific and must always be preserved during upstream merges:

- `tools/entra-auth-provider/**` - Entra ID authentication provider
- `tools/keycloak-auth-provider/**` - Keycloak authentication provider
- `.github/workflows/docker-build-and-push.yml` - GHCR publishing workflow
- `tools/index.yaml` - Custom tool registry (merge manually)

### Upstream-Shared Files (Careful Merging)

These files are modified by both upstream and our fork:

- `Dockerfile` - Registry merge logic requires manual handling
- `chart/values.yaml` - Configuration values need manual review
- `pkg/**` - Core Go code (prefer upstream, test thoroughly)

**Before modifying shared files:**
1. Check if change can be implemented in custom code instead
2. Document why modification is necessary
3. Plan for future upstream merge conflicts

## Authentication Provider Development

### Creating a New Provider

```
tools/
└── your-auth-provider/
    ├── main.go              # HTTP server with OAuth2 endpoints
    ├── tool.gpt             # GPTScript tool definition
    ├── go.mod
    ├── pkg/
    │   ├── profile/         # User profile handling
    │   └── state/           # Session state management
    └── README.md            # Provider documentation
```

### Required Endpoints

| Endpoint | Method | Purpose |
| --------- | -------- | --------- |
| `/{$}` | GET | Return daemon address |
| `/oauth2/start` | GET | Start OAuth2 flow |
| `/oauth2/callback` | GET | Handle OAuth2 callback |
| `/oauth2/sign_out` | GET | Sign out user |
| `/obot-get-state` | POST | Return session state |
| `/obot-get-user-info` | GET | Return user profile |
| `/obot-get-icon-url` | GET | Return profile picture URL |

### Register Provider

**1. Update `tools/index.yaml`:**
```yaml
tools:
  - name: your-auth-provider
    description: Authentication provider for YourService
    modelName: your-auth-provider from github.com/jrmatherly/obot-entraid/tools/your-auth-provider
    localPath: ./your-auth-provider
```

**2. Create `tool.gpt`:**
```gpt
Name: your-auth-provider
Description: Authentication provider for YourService
Credential: ../placeholder-credential

Metadata: envVars: OBOT_YOUR_AUTH_PROVIDER_CLIENT_ID,OBOT_YOUR_AUTH_PROVIDER_CLIENT_SECRET
Metadata: optionalEnvVars: OBOT_YOUR_AUTH_PROVIDER_TENANT_ID

#!/bin/bash
exec your-auth-provider
```

**Reference:** Study `tools/entra-auth-provider/` (complex) or `tools/keycloak-auth-provider/` (simpler) for implementation patterns.

## Testing Your Changes

### Local Testing

```bash
# Backend
make build                   # Build binary
make test                    # Run tests
make lint                    # Lint code

# Frontend
cd ui/user
pnpm run dev                 # Start dev server
pnpm run check               # Type check
pnpm run lint                # Lint
pnpm run ci                  # Run all checks
```

### Docker Build

```bash
# Build Docker image
docker build -t obot-entraid:test .

# Verify auth providers included
docker run --rm obot-entraid:test ls -la /obot-tools/tools/
docker run --rm obot-entraid:test cat /obot-tools/tools/index.yaml | grep -E "(entra|keycloak)"
```

### Integration Testing

```bash
# Run integration tests (if applicable)
make test-integration

# Test authentication flow manually
# (requires setting up test tenant/realm)
```

## Pull Request Process

### Before Submitting

- [ ] Code follows project style (gofmt, ESLint)
- [ ] Tests pass locally (`make test`, `pnpm run ci`)
- [ ] Linting passes (`make lint`, `pnpm run lint`)
- [ ] Build succeeds (`make build`, Docker build)
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow Conventional Commits
- [ ] No sensitive data in commits (credentials, tokens)

### PR Checklist

Our [PR template](.github/PULL_REQUEST_TEMPLATE.md) includes comprehensive checklists for:

- Type of change (bug fix, feature, docs, etc.)
- Testing (local testing, test environment, results)
- Authentication provider changes (if applicable)
- Code quality (formatting, linting, comments)
- Documentation updates
- Deployment compatibility

**Fill out all relevant sections!**

### Review Process

1. **Automated Checks** (~5-10 minutes)
   - Linting and formatting
   - Unit tests
   - Build verification

2. **Maintainer Review** (typically 3-5 business days)
   - Code quality assessment
   - Architecture alignment
   - Security review (for auth changes)

3. **Feedback & Iteration**
   - Address feedback by pushing new commits
   - Re-request review when ready

4. **Merge**
   - Maintainer will merge when approved
   - May squash commits on merge

## Getting Help

### Resources

📖 **Documentation:**
- [Full Contributor Guide](docs/docs/contributing/contributor-guide.md)
- [Auth Provider Guide](tools/README.md)
- [Fork Workflow Analysis](docs/docs/contributing/fork-workflow-analysis-2026.md)

💬 **Communication:**
- [GitHub Discussions](https://github.com/jrmatherly/obot-entraid/discussions) - Ask questions
- [GitHub Issues](https://github.com/jrmatherly/obot-entraid/issues) - Report bugs

🔍 **Examples:**
- Browse `tools/entra-auth-provider/` and `tools/keycloak-auth-provider/`
- Check UI components in `ui/user/src/lib/components/`
- Review tests in `*_test.go` files

### Common Questions

**Q: Do I need to sign a CLA?**
A: No. By submitting a PR, you agree to license your contribution under Apache 2.0.

**Q: Can I work on an assigned issue?**
A: Comment on the issue first to coordinate. If no response after a few days, feel free to proceed.

**Q: My PR has merge conflicts. What do I do?**
```bash
git fetch upstream
git rebase upstream/main
# Resolve conflicts
git push origin feature/your-feature --force-with-lease
```

**Q: Should I squash my commits?**
A: Optional. Maintainers may squash on merge. Focus on clear commit messages.

**Q: Can I submit a work-in-progress PR?**
A: Yes! Mark it as draft or use `[WIP]` prefix. Great for early feedback.

## Code of Conduct

We follow the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).

**Summary:**
- Be respectful and inclusive
- Accept constructive feedback
- Focus on what's best for the community
- Show empathy towards others

## Recognition

Contributors who make significant contributions will be:
- Listed in project CONTRIBUTORS file
- Mentioned in release notes
- Credited in documentation they help create

## License

By contributing to obot-entraid, you agree to license your contributions under the [Apache License 2.0](LICENSE).

---

**Questions?** Open a [Discussion](https://github.com/jrmatherly/obot-entraid/discussions) or comment on an issue.

**Found a bug?** Open an [Issue](https://github.com/jrmatherly/obot-entraid/issues/new).

**Want to chat?** We're friendly! Reach out via discussions.

---

Thank you for contributing to obot-entraid! 🚀
