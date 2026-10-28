---
title: "Encryption and secrets"
---

Obot credentials serve different connections. Manage them according to their owner and use.

| Credential | Configure and revoke |
|---|---|
| Client API key | [Agent authorization scopes](../functionality/agent-auth-scopes.md); deleting a scope invalidates its keys |
| Upstream MCP OAuth application | [Static OAuth](../configuration/mcp-server-oauth-configuration.md); changing application credentials can interrupt use |
| Shared or user-supplied MCP configuration | [vMCP configuration policies](../mcp-gateway/server-types.md#virtual-mcps-configuration-policies) |
| Externally managed Kubernetes Secret | [Secret bindings](../concepts/mcp-hosting.md#mcp-servers-kubernetes-secret-bindings), including the required discovery/resolution label |
| Private image registry credential | [Image Pull Secrets](../configuration/image-pull-secrets.md) |
| Tunnel secret | [Tunnel rotation](../functionality/mcp-tunnels.md#rotate-a-secret); rotation disconnects old clients |

Application-level encryption is disabled by default. Read the [encryption coverage](./credentials.md#overview-encrypted-resources-and-fields) before enabling a provider. It protects selected database and credential fields, not entire records, exported logs, or backups. Existing data may require a separate migration.

Preserve key access when restoring encrypted data; losing the key or KMS permissions can make a database backup unusable.

## Create an agent identity and authorization scope {#agent-auth-scopes-creating-an-agent-authorization-scope}

1. Open **Identity & Access > Agents**.
2. Click **Create Agent Identity**.
3. Fill in the required information:
   - **Name** (required): A descriptive name that identifies the authorization scope's purpose
   - **Description** (optional): Additional context about how the authorization scope is used
   - **Expiration Date** (optional): When the authorization scope should automatically expire. Authorization scopes without an expiration date remain valid until deleted.
   - **MCP Servers**: Select which MCP servers this authorization scope can access. You can:
     - Select **All MCP Servers** to grant access to all servers you currently have access to, including any servers you gain access to in the future
     - Select individual servers to restrict the authorization scope to only those specific servers
     - Leave this empty when you are creating a capability-only scope
   - **API Scopes**: Select any non-MCP capabilities this authorization scope should allow:
     - **LLM proxy access**: Call LLM proxy endpoints
     - **Skill access**: Discover and download skills
     - **Device scan access**: Submit and read device scans
4. Click **Save**.

The UI calls these credentials **agent identities**; each identity has an authorization scope and one or more API keys. For CLI creation and client examples, see the [authorization scope reference](../functionality/agent-auth-scopes.md).

After creation, you'll see a dialog displaying an initial new API key to use. **Copy and save this key immediately**—it will only be shown once and cannot be retrieved later.

## Managing Agent Authorization Scopes {#agent-auth-scopes-managing-agent-authorization-scopes}

### Viewing Your Agent Authorization Scopes {#agent-auth-scopes-viewing-your-agent-authorization-scopes}

Open **Identity & Access > Agents** to see all your agent authorization scopes. The table displays:

| Column | Description |
|--------|-------------|
| Name | The authorization scope's descriptive name |
| Capabilities | Capabilities enabled for the scope; a **Servers** badge indicates MCP server access |
| Last Used | When an API key generated for the scope was last used |
| Expires | When the scope will expire (or "Never" if it has no expiration date) |

### Deleting an Agent Authorization Scope {#agent-auth-scopes-deleting-an-agent-authorization-scope}

1. Open **Identity & Access > Agents**
2. Click the three-dot menu on the authorization scope you want to delete
3. Select **Delete**
4. Confirm the deletion

Deleting an agent authorization scope immediately invalidates its API keys. This action cannot be undone.

## Supported Encryption Providers {#overview-supported-encryption-providers}

Application encryption defaults to `OBOT_SERVER_ENCRYPTION_PROVIDER=none`. Configure one of these providers to enable it:

1. [AWS KMS](../configuration/encryption-providers/aws-kms.md)
2. [Azure Key Vault](../configuration/encryption-providers/azure-key-vault.md)
3. [Google Cloud KMS](../configuration/encryption-providers/google-cloud-kms.md)
4. [Custom](../configuration/encryption-providers/custom-provider.md), including a local AES-GCM key

## How Encryption Works {#overview-how-encryption-works}

Obot uses the Kubernetes `EncryptionConfiguration` format. Each configured resource has a transformer that encrypts selected fields before storage and decrypts them on retrieval. Cloud KMS providers use a local provider process over a Unix socket; a custom AES-GCM configuration uses the supplied key. Encrypted field values are base64-encoded for storage.

The built-in cloud configurations include the resources below. A custom configuration must include the corresponding resource names to protect those fields.

## Encrypted Resources and Fields {#overview-encrypted-resources-and-fields}

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

### MCP and Local-Agent Audit Details {#overview-mcp-and-local-agent-audit-details}

For MCP traffic, the audit transformer encrypts `requestBody`, `mutatedRequestBody`, `responseBody`, `originalResponseBody`, `requestHeaders`, and `responseHeaders`.

For local-agent tool calls in the same audit store, it encrypts the outcome error, hostname, local username, reported user email, working directory, Git root, Git remotes, Git branch, transcript path, request body, response body, and raw event. Other audit metadata, such as timestamps and tool names, is outside this field-level protection.

### Local Passwords {#overview-local-passwords}

Local-auth passwords are stored as salted Argon2id hashes regardless of whether an encryption provider is configured. The password hash is not reversibly encrypted. This differs from upstream passwords stored as credential secret values, which must be recoverable for use.

## Enabling Encryption on an Existing Installation {#overview-enabling-encryption-on-an-existing-installation}

Enabling encryption does not automatically encrypt all existing data. Existing installations may require a separate migration to protect previously stored data.
