# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.2.x   | :white_check_mark: |
| < 0.2   | :x:                |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability in this project, please report it responsibly:

1. **GitHub Security Advisories**: Use the "Report a vulnerability" button in the [Security tab](https://github.com/jrmatherly/obot-entraid/security)
2. **Email**: Contact the repository maintainer directly

### What to Include

When reporting a vulnerability, please provide:

- Type of vulnerability (e.g., SQL injection, XSS, authentication bypass)
- Full path(s) of affected source file(s)
- Location of affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact assessment and potential consequences

### Response Timeline

- **Initial Response**: Within 48 hours (2 business days)
- **Status Update**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues (complex issues may take longer)

## Disclosure Policy

We follow coordinated disclosure practices:

- We will work with you to validate and remediate the vulnerability
- After a fix or mitigation is available, we'll publish release notes
- Security researchers who wish to be acknowledged will be credited in release notes

## Scope

Security issues that impact the confidentiality, integrity, or availability of this project or its official packages/services are in scope.

**Out of scope (non-exhaustive):**
- Vulnerabilities requiring privileged/local access without a clear escalation path
- Deprecated or end-of-life versions
- Vulnerabilities in third-party dependencies not owned by this project (please report upstream)

## Safe Harbor

We will not pursue legal action against security researchers conducting good-faith research aligned with this policy.

Please avoid:
- Privacy violations
- Service degradation or denial of service
- Data destruction or corruption
- Testing against accounts or data you do not own

Only test against your own accounts and data.

## Receiving Security Fixes

Security fixes are shipped in patch releases. We recommend upgrading to the latest patch version of supported releases.

We may issue public advisories (GitHub Security Advisory/CVE) when appropriate.

## Security Best Practices

This project follows security best practices including:

- **Code scanning**: GitHub CodeQL for static analysis
- **Dependency scanning**: Renovate for automated vulnerability detection
- **Container scanning**: Trivy for container image vulnerability scanning
- **Image signing**: Cosign (Sigstore) for container image signing
- **Least-privilege Actions**: GitHub Actions workflows use minimal permissions
- **Security headers**: Appropriate HTTP security headers in production
- **Encrypted secrets**: All sensitive configuration encrypted at rest

## Auth Provider Security

The custom authentication providers (Entra ID, Keycloak) in this fork implement security best practices:

### OAuth 2.0/OIDC Standards
- Standard authorization code flow (OAuth 2.0)
- OpenID Connect (OIDC) for identity layer
- PKCE (Proof Key for Code Exchange) where supported

### Token Security
- Access tokens stored in encrypted HTTP-only cookies
- Cookie encryption using `OBOT_AUTH_PROVIDER_COOKIE_SECRET`
- Tokens never exposed to client-side JavaScript
- Secure flag enabled for HTTPS deployments
- SameSite cookie protection

### Profile Data Handling
- Profile pictures returned as base64 data URLs (not external API URLs requiring authentication)
- Prevents browser 401 errors and token leakage
- No third-party requests from user browsers to auth provider APIs

### Required Environment Variables
```bash
OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID      # Azure App Registration Client ID
OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET  # Azure App Registration Secret (encrypted)
OBOT_AUTH_PROVIDER_COOKIE_SECRET        # Base64-encoded encryption key for cookies
OBOT_AUTH_PROVIDER_EMAIL_DOMAINS        # Allowed email domains (default: "*")
```

### Audit and Compliance
- All authentication events logged
- Session management with configurable timeouts
- Support for admin-level access control
- Integration with Kubernetes RBAC when deployed in-cluster

## Credits

With permission from reporters, we will credit security researchers in release notes and acknowledge their contributions to improving the security of this project.

Thank you for helping keep obot-entraid and its users safe!
