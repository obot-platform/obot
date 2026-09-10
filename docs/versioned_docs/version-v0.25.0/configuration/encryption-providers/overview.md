# Overview

Obot can encrypt selected sensitive fields in its database and credential store. **Application-level encryption is disabled by default** (`OBOT_SERVER_ENCRYPTION_PROVIDER=none`). Configure a provider to enable it. This protects the fields listed below, not entire records, database files, backups, or exported audit logs.

## Supported Encryption Providers

1. [AWS KMS](./aws-kms.md)
2. [Azure Key Vault](./azure-key-vault.md)
3. [Google Cloud KMS](./google-cloud-kms.md)
4. [Custom](./custom-provider.md), including a local AES-GCM key

## How Encryption Works

Obot uses the Kubernetes `EncryptionConfiguration` format. Each configured resource has a transformer that encrypts selected fields before storage and decrypts them on retrieval. Cloud KMS providers use a local provider process over a Unix socket; a custom AES-GCM configuration uses the supplied key. Encrypted field values are base64-encoded for storage.

The built-in cloud configurations include the resources below. A custom configuration must include the corresponding resource names to protect those fields.

## Encrypted Resources and Fields

These names identify encryption transformers; they do not mean every field in the resource is encrypted. IDs, timestamps, and other metadata can remain readable.

| Resource | Selected fields protected when configured |
|----------|-------------------------------------------|
| `credentials.obot.obot.ai` | Serialized credential secret values, such as API keys and upstream access tokens; credential names and context metadata are not included |
| `users.obot.obot.ai` | `username`, `email`, `displayName`, `iconURL`, `originalEmail`, `originalUsername`; local-auth user `email` also uses this transformer |
| `identities.obot.obot.ai` | `providerUsername`, `email`, `providerUserID`, `providerGroupLookupID`, `iconURL` |
| `mcpoauthtokens.obot.obot.ai` | `accessToken`, `refreshToken`, `clientID`, `clientSecret` |
| `mcpoauthpendingstates.obot.obot.ai` | `state`, `verifier`, `clientID`, `clientSecret` |
| `mcpauditlogs.obot.obot.ai` | MCP request and response bodies and headers, including mutated requests and original responses; local-agent details listed below |
| `llmauditlogs.obot.obot.ai` | Request and response headers and bodies, including `policyModifiedRequestBody` |
| `policyviolations.obot.obot.ai` | `blockedContent` |
| `properties.obot.obot.ai` | Stored property `value` |

### MCP and Local-Agent Audit Details

For MCP traffic, the audit transformer encrypts `requestBody`, `mutatedRequestBody`, `responseBody`, `originalResponseBody`, `requestHeaders`, and `responseHeaders`.

For local-agent tool calls in the same audit store, it encrypts the outcome error, hostname, local username, reported user email, working directory, Git root, Git remotes, Git branch, transcript path, request body, response body, and raw event. Other audit metadata, such as timestamps and tool names, is outside this field-level protection.

### Local Passwords

Local-auth passwords are stored as salted Argon2id hashes regardless of whether an encryption provider is configured. The password hash is not reversibly encrypted. This differs from upstream passwords stored as credential secret values, which must be recoverable for use.

## Enabling Encryption on an Existing Installation

Enabling encryption does not automatically encrypt all existing data. Existing installations may require a separate migration to protect previously stored data.
