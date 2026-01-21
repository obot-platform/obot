# Overview

Obot supports encrypting sensitive data at rest in the database using industry-standard encryption providers. When enabled, encryption protects user data, credentials, OAuth tokens, session information, and other sensitive fields using external Key Management Services (KMS).

## Supported Encryption Providers

Obot supports the following encryption providers:

1. [AWS KMS](./aws-kms.md)
2. [Azure Key Vault](./azure-key-vault.md)
3. [Google Cloud KMS](./google-cloud-kms.md)
4. [Custom](./custom-provider.md)

## How Encryption Works

Obot uses the Kubernetes EncryptionConfiguration format to encrypt data at rest. The encryption provider:

1. Receives data to encrypt via a Unix socket connection
2. Encrypts the data using the configured KMS provider
3. Returns the encrypted data to be stored in the database
4. On retrieval, decrypts the data before returning it to the application

All encrypted string fields are base64-encoded after encryption for safe storage.

## Encrypted Resources

When you enable an encryption provider, the following resource types are automatically encrypted:

| Resource Type | Description |
|---------------|-------------|
| `credentials` | Credential store data |
| `runstates.obot.obot.ai` | Run state data for agent executions |
| `users.obot.obot.ai` | User account information |
| `identities.obot.obot.ai` | Identity provider data |
| `mcpoauthtokens.obot.obot.ai` | MCP OAuth tokens |
| `mcpauditlogs.obot.obot.ai` | MCP audit log data |
| `sessioncookies.obot.obot.ai` | Session cookie data |

## Complete List of Encrypted Fields

### User Data (`users.obot.obot.ai`)
All personal user information is encrypted:
- `username` - User's username
- `email` - User's email address
- `displayName` - User's display name
- `iconURL` - User's profile icon URL
- `originalEmail` - Original email for a deleted user from identity provider
- `originalUsername` - Original username for a deleted user from identity provider

### Identity Data (`identities.obot.obot.ai`)
Identity provider information is encrypted:
- `providerUsername` - Username from identity provider
- `email` - Email from identity provider
- `providerUserID` - User ID from identity provider
- `iconURL` - Icon URL from identity provider

### MCP OAuth Tokens (`mcpoauthtokens.obot.obot.ai`)
All OAuth-related secrets are encrypted:
- `accessToken` - OAuth access token
- `refreshToken` - OAuth refresh token
- `clientID` - OAuth client ID
- `clientSecret` - OAuth client secret
- `state` - OAuth state parameter
- `verifier` - PKCE verifier

### Session Cookies (`sessioncookies.obot.obot.ai`)
Session authentication data is encrypted:
- `cookie` - Session cookie value

### MCP Audit Logs (`mcpauditlogs.obot.obot.ai`)
Complete request/response data in audit logs is encrypted:
- `requestBody` - HTTP request body (JSON)
- `responseBody` - HTTP response body (JSON)
- `requestHeaders` - HTTP request headers (JSON)
- `responseHeaders` - HTTP response headers (JSON)

### Run State Data (`runstates.obot.obot.ai`)
Agent execution state data is encrypted:
- `output` - Execution output (binary)
- `callFrame` - Call frame data (binary)
- `chatState` - Chat state data (binary)

### Credentials (`credentials`)
All credential data stored via the credential store system (SQLite or PostgreSQL backend) is encrypted.

The credential store is configured with the encryption provider and automatically encrypts all stored credentials, including:
- API keys
- Access tokens
- Passwords
- Any other sensitive credential or configuration data
