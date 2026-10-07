---
draft: true
title: "Configure agent credentials"
---

Choose the credential setup for the agent experience you are using. Credentials identify the caller; resource grants determine what it can use.

| Agent experience | Credential setup |
|---|---|
| Obot Agent | The user signs in to Obot. An administrator configures model-provider credentials; the user completes required MCP connection configuration. |
| External client or automation | Use the client's supported Obot OAuth flow or create an agent identity with a scoped API key. |

## Obot Agent

1. Sign in with the intended user's account.
2. Confirm that a [model access policy](../functionality/model-access-policies.md) grants an active model from a configured provider. Users do not need the provider's upstream API key.
3. Add the required MCP resources to the project and complete any requested connection fields or remote-service OAuth authorization.
4. Test a small model request and a read-only tool under that user account before scheduling work.

Obot supplies configured provider credentials or the user's upstream OAuth token when forwarding requests. The user's Obot sign-in is separate from authentication to a remote MCP service.

## External clients and automation

For interactive MCP clients that support OAuth, follow [Connect AI clients](../mcp-gateway/connect-clients.md). For API-key clients:

1. Open **Identity & Access > Agents** and select **Create Agent Identity**.
2. Name the identity, choose an expiration, and select only the required MCP resources and API capabilities. Enable **LLM proxy access** when the client needs model requests.
3. Save and copy the initial API key. Store it in the client's supported secret storage; it is shown only once.
4. Configure the copied vMCP URL or provider gateway URL using the [MCP client](../mcp-gateway/connect-clients.md#clients-using-api-keys) or [LLM client](../llm-gateway/connect-clients.md) instructions.
5. Verify a request with that key. The identity cannot grant access beyond its owner.

Deleting the identity's authorization scope invalidates its keys. See [Agent authorization scopes](../functionality/agent-auth-scopes.md) for key testing, expiration, and client examples.

## Shared and personal upstream credentials

An Obot sign-in or client API key authenticates to Obot. The MCP component may still need a separate upstream credential. Use [vMCP configuration policies](../mcp-gateway/server-types.md#virtual-mcps-configuration-policies) to choose values shared by the vMCP or supplied by each connecting user.

For credential storage protection and rotation, see [Encryption and secrets](../security/credentials.md).
