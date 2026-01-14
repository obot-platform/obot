## Summary

<!-- Brief description of changes - what does this PR do? -->

## Type of Change

<!-- Mark relevant options with 'x' -->

- [ ] 🐛 Bug fix (non-breaking change fixing an issue)
- [ ] ✨ New feature (non-breaking change adding functionality)
- [ ] 💥 Breaking change (fix or feature causing existing functionality to break)
- [ ] 📚 Documentation update
- [ ] 🔧 Configuration/dependency update
- [ ] ⚡ Performance improvement
- [ ] ♻️ Code refactoring
- [ ] 🧪 Test addition/modification
- [ ] 🚀 CI/CD or infrastructure change

## Related Issues

<!-- Link related issues using keywords: Closes #123, Fixes #456, Relates to #789 -->

Closes #
Relates to #

## Changes Made

<!-- List specific changes in bullet points -->

-
-
-

## Testing

### Local Testing

- [ ] Tests pass (`make test`)
- [ ] Linting passes (`make lint`)
- [ ] Build succeeds (`make build`)
- [ ] New tests added for new functionality
- [ ] Manual testing performed

### Test Environment

- OS: <!-- e.g., macOS 14, Ubuntu 22.04 -->
- Go Version: <!-- e.g., 1.25.5 -->
- Node/pnpm: <!-- e.g., 20.10/9.1 (if UI changes) -->

### Test Results

<details>
<summary>Click to expand test output</summary>

```bash
# Paste relevant test output here
```

</details>

## Authentication Provider Changes

<!-- Complete if changes affect auth providers (entra-auth-provider, keycloak-auth-provider) -->

- [ ] No auth provider changes
- [ ] Existing provider modified: <!-- specify which -->
- [ ] New provider added: <!-- specify name -->

### Provider-Specific Checklist

<!-- If auth provider changes, verify these -->

- [ ] All required endpoints implemented (`/`, `/oauth2/*`, `/obot-get-*`)
- [ ] Tool registry (`tools/index.yaml`) updated
- [ ] Field mapping correct (`name`, `icon_url` as base64 data URL)
- [ ] Token cookie named `obot_access_token` with proper encryption
- [ ] `tool.gpt` declares `envVars` and `optionalEnvVars` metadata
- [ ] Provider documentation added/updated in `tools/README.md`
- [ ] Dockerfile includes provider correctly
- [ ] Tested with actual authentication flow (if possible)

## Code Quality

### General

- [ ] Code follows [project conventions](docs/docs/contributing/contributor-guide.md)
- [ ] Self-reviewed all changes
- [ ] Comments added for complex logic
- [ ] No debug statements or console.log left in code
- [ ] No sensitive data (tokens, passwords, secrets) in code or commits

### Go-Specific

- [ ] `gofmt` applied
- [ ] `golangci-lint` passes
- [ ] Exported functions/types have GoDoc comments
- [ ] Proper error handling (no ignored errors)
- [ ] No panics in production code paths

### UI-Specific (if applicable)

- [ ] ESLint passes (`pnpm run lint`)
- [ ] TypeScript type-checks (`pnpm run check`)
- [ ] Components follow project structure
- [ ] Accessible UI (ARIA labels, keyboard navigation)

## Documentation

- [ ] README updated (if needed)
- [ ] API documentation updated (if applicable)
- [ ] Code comments added for public APIs
- [ ] Migration guide provided (if breaking change)
- [ ] Environment variables documented (if new/changed)

### New Environment Variables

<!-- If adding/changing environment variables, document them -->

| Variable | Required | Default | Description |
| --------- | --------- | --------- | ------------- |
| <!-- `OBOT_...` --> | <!-- Yes/No --> | <!-- value --> | <!-- Description --> |

## Deployment & Compatibility

- [ ] Backward compatible with existing deployments
- [ ] No database migrations required
- [ ] Helm chart updated (if infrastructure changes)
- [ ] Works with current Kubernetes deployment

### Upstream Merge Compatibility

<!-- Important for maintaining the fork -->

- [ ] Changes are in custom code only (tools/, workflows, fork-specific)
- [ ] Changes touch shared code (may conflict with future upstream merges)

**If shared code changed:**
- Conflict resolution strategy: <!-- describe how to handle future upstream conflicts -->
- Documented in: <!-- link to relevant documentation -->

## Breaking Changes

- [ ] No breaking changes

**If breaking changes exist:**

**Description:**
<!-- Describe what breaks and why -->

**Migration Path:**
<!-- Step-by-step guide for users to migrate -->

1.
2.
3.

## Performance Impact

- [ ] No performance impact expected
- [ ] Performance improved (describe below)
- [ ] May impact performance (explain and justify)

**Details:**
<!-- If performance changed, provide benchmarks or analysis -->

## Security Considerations

- [ ] No security implications
- [ ] Secrets/credentials handling reviewed
- [ ] Input validation added where needed
- [ ] Authentication/authorization logic secure
- [ ] Dependencies don't introduce known vulnerabilities

## Screenshots/Videos

<!-- For UI changes, show before/after -->

### Before

<!-- Screenshot or remove this section -->

### After

<!-- Screenshot or remove this section -->

## Additional Context

<!-- Any other information reviewers should know -->

## Post-Merge Actions

<!-- Actions needed after merging (if any) -->

- [ ] No post-merge actions required
- [ ] Update configuration/secrets
- [ ] Run database migrations
- [ ] Announce changes to users
- [ ] Update deployment documentation

---

## Reviewer Checklist

<!-- For maintainers reviewing this PR -->

- [ ] Code quality meets standards
- [ ] Tests comprehensive and passing
- [ ] Documentation clear and complete
- [ ] Security considerations addressed
- [ ] Breaking changes properly handled
- [ ] Performance acceptable
- [ ] Deployment plan sound

---

**Thank you for contributing to obot-entraid!** 🚀

By submitting this PR, you agree to license your contribution under the Apache 2.0 license.

📖 **New to contributing?** Check out our [Contributor Guide](docs/docs/contributing/contributor-guide.md).
