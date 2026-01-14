# Contributor Guide

Thank you for considering contributing to obot-entraid! This guide will help you submit high-quality contributions to our fork of obot-platform/obot with custom authentication providers.

## Quick Start

### Prerequisites

- **Go**: 1.25.5 or later
- **Node.js**: 18+ (for UI development)
- **pnpm**: Latest version
- **Docker**: For testing container builds
- **Git**: For version control
- **GitHub CLI** (optional): `gh` for easier workflow management

### Fork and Clone

1. **Fork this repository** to your GitHub account
   - Visit https://github.com/jrmatherly/obot-entraid
   - Click "Fork" button

2. **Clone your fork:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/obot-entraid.git
   cd obot-entraid
   ```

3. **Add upstream remote:**
   ```bash
   git remote add upstream https://github.com/jrmatherly/obot-entraid.git
   git fetch upstream
   ```

4. **Verify remotes:**
   ```bash
   git remote -v
   # origin    https://github.com/YOUR_USERNAME/obot-entraid.git (fetch)
   # origin    https://github.com/YOUR_USERNAME/obot-entraid.git (push)
   # upstream  https://github.com/jrmatherly/obot-entraid.git (fetch)
   # upstream  https://github.com/jrmatherly/obot-entraid.git (push)
   ```

---

## Development Workflow

### 1. Create a Feature Branch

Always create a feature branch from the latest `main`:

```bash
# Update your main branch
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/your-feature-name

# Branch naming conventions:
# - feature/add-saml-auth         (new features)
# - fix/entra-token-refresh       (bug fixes)
# - docs/update-keycloak-guide    (documentation)
# - refactor/auth-provider-common (refactoring)
# - test/integration-test-suite   (tests)
```

### 2. Make Your Changes

Follow our coding standards:

**Go Code:**
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Run `golangci-lint` before committing
- Write tests for new functionality
- Document exported functions and types

**Svelte/TypeScript:**
- Follow project ESLint configuration
- Use TypeScript for type safety
- Follow component structure in `ui/user/src/lib/components/`
- Write unit tests for complex logic

**Commit Messages:**
Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, no logic change)
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `perf`: Performance improvement
- `test`: Adding or updating tests
- `chore`: Maintenance tasks (dependencies, build scripts)
- `ci`: CI/CD changes

**Examples:**

```bash
# Good commit messages:
git commit -m "feat(entra): add conditional access policy support

Implements Microsoft Entra Conditional Access policy evaluation
during authentication flow.

- Adds CAP policy API client
- Implements policy evaluation middleware
- Updates auth flow to handle CAP challenges

Closes #45"

git commit -m "fix(keycloak): resolve token expiration handling

Token expiration was not being properly detected, causing
authentication errors after 1 hour.

- Add token expiration check in middleware
- Implement automatic refresh flow
- Add tests for expiration scenarios

Fixes #78"

git commit -m "docs: update Entra ID setup guide with MFA configuration"
```

### 3. Test Your Changes

Run all relevant tests before submitting:

**Go Backend:**
```bash
# Lint
make lint

# Build
make build

# Run tests
make test

# Run integration tests (if applicable)
make test-integration
```

**Svelte UI:**
```bash
cd ui/user

# Install dependencies (first time)
pnpm install

# Type checking
pnpm run check

# Linting
pnpm run lint

# Format check
pnpm run format

# Run all checks
pnpm run ci
```

**Docker Build:**
```bash
# Test Docker build (important for auth providers)
docker build -t obot-entraid:test .

# Verify tools in container
docker run --rm obot-entraid:test ls -la /obot-tools/tools/
docker run --rm obot-entraid:test cat /obot-tools/tools/index.yaml | grep -E "(entra|keycloak)"
```

### 4. Keep Your Branch Updated

Regularly sync with upstream to avoid conflicts:

```bash
# Fetch latest changes
git fetch upstream

# Rebase your branch (for feature branches not yet in PR)
git rebase upstream/main

# Or merge (if you prefer or if branch is shared)
git merge upstream/main

# Resolve any conflicts
# After resolving: git add <files> && git rebase --continue
```

### 5. Interactive Rebase (Optional but Recommended)

Clean up your commit history before submitting:

```bash
# View your commits since branching from main
git log --oneline main..HEAD

# Start interactive rebase
git rebase -i main

# In the editor, use these commands:
# - pick (p): keep commit as-is
# - squash (s): merge with previous commit
# - fixup (f): merge and discard commit message
# - reword (r): edit commit message
# - drop (d): remove commit

# Example: squash fixup commits
pick abc1234 feat(auth): add OAuth refresh support
squash def5678 fix typo in comments
squash ghi9012 address review feedback
pick jkl3456 docs: update auth provider guide

# Save and close editor
# Git will combine squashed commits and let you edit the message
```

### 6. Push to Your Fork

```bash
# First push
git push origin feature/your-feature-name

# After rebase (if you've already pushed)
git push origin feature/your-feature-name --force-with-lease
```

**Important:** Only use `--force-with-lease` (never plain `--force`). This ensures you don't accidentally overwrite changes from others.

### 7. Submit Pull Request

**Via GitHub CLI:**
```bash
gh pr create --base main --head feature/your-feature-name \
  --title "feat(entra): Add MFA support for Entra ID authentication" \
  --body "## Summary
- Implements MFA challenge/response flow
- Adds conditional access policy support
- Updates tests for MFA scenarios

## Testing
- Tested against Entra ID tenant with MFA enabled
- All existing auth tests pass

## Checklist
- [x] Tests added/updated
- [x] Documentation updated
- [x] Follows code style
- [x] Passes CI checks"
```

**Via GitHub Web UI:**
1. Go to your fork on GitHub
2. Click "Compare & pull request" button
3. Fill out the PR template
4. Submit for review

---

## Code Review Process

### What to Expect

1. **Automated Checks** (~5-10 minutes)
   - Linting (Go, Svelte)
   - Tests (unit, integration)
   - Build verification (Docker)
   - All must pass before review

2. **Maintainer Review** (typically 3-5 business days)
   - Code quality assessment
   - Architecture alignment check
   - Security review (for auth changes)
   - Testing adequacy verification

3. **Feedback & Iteration**
   - Address feedback by pushing new commits to your branch
   - PR automatically updates with new commits
   - Re-request review when ready

4. **Merge**
   - Maintainer will merge when approved
   - May squash commits on merge
   - PR automatically closes linked issues

### What We Look For

✅ **Clear, focused changes** (one feature/fix per PR)
✅ **Comprehensive tests** for new functionality
✅ **Documentation updates** where applicable
✅ **Follows existing code patterns** and style
✅ **Passes all CI checks**
✅ **Clear commit messages** (Conventional Commits)
✅ **No unrelated changes** (formatting, refactoring in feature PRs)

❌ **Large, monolithic PRs** (split into smaller ones)
❌ **Missing tests** for new features
❌ **Breaking changes** without discussion
❌ **Unformatted code** (run formatters)
❌ **Unclear purpose** (describe what and why)

---

## Areas We'd Love Help With

### High Priority

🔐 **Additional Authentication Providers**
- LDAP/Active Directory
- SAML 2.0
- Okta
- Auth0
- Generic OIDC

📚 **Documentation Improvements**
- Setup guides for different environments
- Troubleshooting guides
- Architecture documentation
- API documentation

🧪 **Test Coverage Expansion**
- Integration tests for auth providers
- E2E tests for complete flows
- Load testing for auth endpoints

### Medium Priority

🐛 **Bug Fixes**
- Check [Issues](https://github.com/jrmatherly/obot-entraid/issues) for bugs
- Improve error messages
- Edge case handling

🌐 **Internationalization (i18n)**
- UI translations
- Error message translations
- Documentation translations

⚡ **Performance Improvements**
- Auth provider response time optimization
- Caching strategies
- Database query optimization

### Ongoing

✨ **Feature Enhancements**
- Multi-factor authentication improvements
- Session management enhancements
- Audit logging improvements

---

## Authentication Provider Development

### Creating a New Auth Provider

If you're adding a new authentication provider, follow this structure:

**1. Directory Structure:**
```
tools/
└── your-auth-provider/
    ├── main.go              # HTTP server implementation
    ├── tool.gpt             # GPTScript tool definition
    ├── go.mod               # Go module definition
    ├── pkg/
    │   ├── profile/
    │   │   └── profile.go   # User profile handling
    │   └── state/
    │       └── state.go     # Session state management
    └── README.md            # Provider-specific documentation
```

**2. Required Endpoints:**

Your auth provider must implement these endpoints:

| Endpoint | Method | Purpose |
| --------- | -------- | --------- |
| `/{$}` | GET | Return daemon address (`http://127.0.0.1:<port>`) |
| `/oauth2/start` | GET | Start OAuth2 flow |
| `/oauth2/callback` | GET | Handle OAuth2 callback |
| `/oauth2/sign_out` | GET | Sign out user |
| `/obot-get-state` | POST | Return user session state |
| `/obot-get-user-info` | GET | Return user profile |
| `/obot-get-icon-url` | GET | Return profile picture URL |

**3. Tool Definition (tool.gpt):**

```gpt
Name: your-auth-provider
Description: Authentication provider for YourService
Credential: ../placeholder-credential

Metadata: envVars: OBOT_YOUR_AUTH_PROVIDER_CLIENT_ID,OBOT_YOUR_AUTH_PROVIDER_CLIENT_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_EMAIL_DOMAINS
Metadata: optionalEnvVars: OBOT_YOUR_AUTH_PROVIDER_TENANT_ID,OBOT_YOUR_AUTH_PROVIDER_CUSTOM_SCOPE

#!/bin/bash
exec your-auth-provider
```

**4. Register in Tool Registry:**

Add your provider to `tools/index.yaml`:

```yaml
tools:
  - name: your-auth-provider
    description: Authentication provider for YourService
    modelName: your-auth-provider from github.com/jrmatherly/obot-entraid/tools/your-auth-provider
    localPath: ./your-auth-provider
```

**5. Update Dockerfile:**

The Dockerfile automatically includes tools from `tools/` directory, but verify the merge logic works:

```dockerfile
# Should automatically pick up your provider
COPY tools/ /obot-tools-custom/
```

**6. Reference Implementation:**

Study existing providers:
- **Entra ID**: `tools/entra-auth-provider/` (complex, with profile pictures)
- **Keycloak**: `tools/keycloak-auth-provider/` (OIDC standard)

**7. Testing Your Provider:**

```bash
# Build locally
cd tools/your-auth-provider
go build .

# Test endpoints
./your-auth-provider &
PID=$!

curl http://localhost:YOUR_PORT/
curl -H "Authorization: Bearer test" http://localhost:YOUR_PORT/obot-get-user-info

kill $PID
```

**8. Documentation:**

Create comprehensive documentation:
- Setup guide (provider configuration)
- Environment variables reference
- Common issues and troubleshooting
- Example configurations

---

## Getting Help

### Resources

📖 **Documentation:**
- [Main Documentation](https://github.com/jrmatherly/obot-entraid/tree/main/docs)
- [Upstream Merge Process](upstream-merge-process.md)
- [Fork Workflow Analysis](fork-workflow-analysis-2026.md)
- [Local Development Guide](local-development.md)

💬 **Communication:**
- [GitHub Discussions](https://github.com/jrmatherly/obot-entraid/discussions) - Ask questions
- [GitHub Issues](https://github.com/jrmatherly/obot-entraid/issues) - Report bugs

🔍 **Code Examples:**
- Browse existing auth providers in `tools/`
- Check UI components in `ui/user/src/lib/components/`
- Review tests in `*_test.go` files

### Common Questions

**Q: Do I need to sign a CLA?**
A: No, we don't require a CLA. By submitting a PR, you agree to license your contribution under the project's license (Apache 2.0).

**Q: Can I work on an issue that's already assigned?**
A: It's best to comment on the issue first to coordinate. If no response after a few days, feel free to proceed.

**Q: My PR has merge conflicts. What do I do?**
A: Rebase your branch on the latest main:
```bash
git fetch upstream
git rebase upstream/main
# Resolve conflicts
git push origin feature/your-feature --force-with-lease
```

**Q: How do I update my fork's main branch?**
A: Sync with upstream:
```bash
git checkout main
git fetch upstream
git merge upstream/main --ff-only
git push origin main
```

**Q: Should I squash my commits before submitting?**
A: Optional. Maintainers may squash on merge anyway. Focus on clear commit messages.

**Q: Can I submit a work-in-progress PR?**
A: Yes! Use `[WIP]` or draft PR feature. Great for early feedback.

---

## Advanced Topics

### Interactive Rebase Deep Dive

Interactive rebase is powerful for cleaning up commit history:

```bash
# View commits to rebase
git log --oneline main..HEAD

# Start interactive rebase
git rebase -i main

# Commands available in editor:
# pick    = use commit
# reword  = use commit, but edit message
# edit    = use commit, but stop for amending
# squash  = use commit, but merge into previous
# fixup   = like squash, but discard message
# drop    = remove commit

# Example workflow:
# Initial commits:
#   abc1234 feat: add feature
#   def5678 fix typo
#   ghi9012 fix another typo
#   jkl3456 add tests
#
# After rebase:
pick abc1234 feat: add feature with tests
fixup def5678 fix typo
fixup ghi9012 fix another typo
squash jkl3456 add tests
```

### Testing Strategies

**Unit Tests:**
```go
// tools/your-auth-provider/pkg/profile/profile_test.go
func TestFetchUserProfile(t *testing.T) {
    // Mock HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "id": "12345",
            "name": "Test User",
        })
    }))
    defer server.Close()

    // Test your function
    profile, err := FetchUserProfile(context.Background(), "token", server.URL)
    assert.NoError(t, err)
    assert.Equal(t, "Test User", profile["name"])
}
```

**Integration Tests:**
```bash
# Run integration tests
make test-integration

# Or specific test
go test -tags=integration ./tests/integration/...
```

### Debugging Tips

**Go Backend:**
```bash
# Enable verbose logging
export OBOT_LOG_LEVEL=debug

# Run with delve debugger
dlv debug ./main.go -- server
```

**Docker Build:**
```bash
# Build with specific target
docker build --target builder -t obot-entraid:builder .

# Inspect layers
docker history obot-entraid:test

# Run with shell
docker run -it --entrypoint /bin/sh obot-entraid:test
```

---

## Code of Conduct

We follow the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you agree to uphold this code.

**Summary:**
- Be respectful and inclusive
- Accept constructive feedback
- Focus on what's best for the community
- Show empathy towards others

---

## Recognition

Contributors who make significant contributions will be:
- Listed in project CONTRIBUTORS file
- Mentioned in release notes
- Credited in documentation they help create

Thank you for contributing to obot-entraid! 🚀

---

**Questions?** Open a [Discussion](https://github.com/jrmatherly/obot-entraid/discussions) or comment on an issue.

**Found a bug?** Open an [Issue](https://github.com/jrmatherly/obot-entraid/issues/new).

**Want to chat?** We're friendly! Reach out via discussions.
