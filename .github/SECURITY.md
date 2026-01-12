# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.2.x   | :white_check_mark: |
| < 0.2   | :x:                |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability in this project, please report it responsibly:

1. **Email**: Send details to the repository maintainer
2. **GitHub Security Advisories**: Use the "Report a vulnerability" button in the Security tab

### What to Include

- Type of vulnerability (e.g., SQL injection, XSS, authentication bypass)
- Full path(s) of affected source file(s)
- Step-by-step instructions to reproduce
- Proof-of-concept or exploit code (if possible)
- Impact assessment

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues

## Security Best Practices

This project follows security best practices including:

- Code scanning with GitHub CodeQL
- Dependency vulnerability scanning with Renovate
- Container image scanning with Trivy
- Image signing with Cosign (Sigstore)
- Least-privilege GitHub Actions permissions

## Auth Provider Security

The custom authentication providers (Entra ID, Keycloak) in this fork:

- Use OAuth 2.0/OIDC standard flows
- Store tokens in encrypted HTTP-only cookies
- Never expose access tokens to client-side JavaScript
- Return profile pictures as base64 data URLs (not external API URLs requiring auth)
