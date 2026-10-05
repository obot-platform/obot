---
title: "Troubleshooting"
---

Start with the request's Obot provider endpoint, requested model, user, and time. An administrator or auditor can use **Operations > Audit Logs > Model** to correlate the request with its recorded outcome. Remove API keys and sensitive prompts before sharing diagnostics.

## Authentication fails

Confirm that the client sends an Obot API key with **LLM proxy access**, using the authentication format documented for that client and provider route. An upstream provider key does not authenticate the client to Obot. An expired or deleted authorization scope no longer grants access.

Check the key against `/api/me` as described in [Testing an API key](../functionality/agent-auth-scopes.md#testing-an-api-key). A successful result verifies the key's identity; it does not prove the key has LLM proxy access or that its owner has model grants.

## No models appear or a model is denied

1. Open **Models** as the affected user and check which models are available.
2. Ask an administrator to check the matching policies under **Models > Access Policies**. Administrators also need model grants.
3. Check that the provider is configured and the intended model is active under **Models > Model Providers**.
4. Use the provider model ID shown in Obot. For Azure, use the configured deployment name.

A configured provider does not grant every user access. See [Control model access](../functionality/model-access-policies.md).

## An alias fails or uses an unexpected model

`llm` and `llm-mini` refer to global defaults. An administrator can inspect their targets under **Models > Model Providers > Set Default Models**.

The alias must point to an active model, the caller must have access to that model, and the request must use the matching provider route. For example, an alias pointing to an Anthropic model cannot be sent to the OpenAI route. If the alias target changes providers, update the client's base URL. See [Model aliases](./how-it-works.md#using-model-aliases).

## The endpoint or request format is rejected

Check the client's base URL against the example on **Models**. Use the route for the configured provider and the API format it supports. The OpenAI-compatible gateway routes support the Responses API; they do not support Chat Completions. Azure deployment names do not determine whether a request uses the Anthropic or OpenAI format.

For model discovery differences and client-specific settings, use [API compatibility](./compatibility.md) and [Connect clients and applications](./connect-clients.md).

## The upstream provider rejects the request

An administrator should inspect the provider configuration, upstream credentials, requested model or deployment, and any upstream quota or rate-limit error. Check network access from Obot to the provider. Repeatedly changing the user's Obot key will not repair an invalid upstream credential.

If a configured [AI Judge policy](../functionality/ai-judge-policies.md) blocks a request, inspect that policy's outcome before changing model grants. Policy enforcement and provider errors are different causes.

## Verify the repair

Repeat a small request using the same client route and affected user's access. Check the returned model and the new audit outcome. A successful request under an administrator's account does not establish another user's access. For an unreachable Obot service, continue to [deployment troubleshooting](../operations/troubleshooting.md).
