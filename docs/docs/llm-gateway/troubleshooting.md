---
title: "Troubleshooting"
---

Start with the request's Obot provider endpoint, requested model, user, and time. An administrator or auditor can use **Operations > Audit Logs > LLM** to correlate the request with its recorded outcome. Remove API keys and sensitive prompts before sharing diagnostics.

## Authentication fails

Confirm that the client sends an Obot API key with **LLM proxy access**, using the authentication format documented for that client. An upstream provider key does not authenticate the client to Obot. An expired or deleted authorization scope no longer grants access.

Check the key against `/api/me` as described in [Testing an API key](../functionality/agent-auth-scopes.md#testing-an-api-key). A successful result verifies the key's identity; it does not prove the key has LLM proxy access or that its owner has model grants.

## No models appear or a model is denied

1. Open **LLM Gateway** as the affected user and check which models are available.
2. Ask an administrator to check the matching policies under **LLM Gateway > Profiles**. Administrators also need model grants.
3. Check that the provider is configured under **LLM Gateway > Providers** and that the intended model is listed under **Models** on the provider's page. If it's missing, select **Refresh models**.
4. Use the model name shown on **LLM Gateway**, in `provider/model` form, such as `anthropic/claude-sonnet-4-5`.

A configured provider does not grant every user access. See [Control model access](../functionality/model-access-policies.md).

## An alias fails or uses an unexpected model

`llm` and `llm-mini` refer to global defaults. An administrator can inspect their targets with **Set Default Models** under **LLM Gateway > Providers**.

The alias must point to an active model, and the caller must have access to that model. See [Default models](./how-it-works.md#default-models).

## The endpoint or request format is rejected

Check the client's base URL against the example on **LLM Gateway**. The gateway serves the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats at one base URL for every model. Requests to a former per-provider route other than OpenAI and Anthropic get `410 Gone` with the new URL.

For client-specific settings and known limits, see [Using with Claude Code](./connect-clients.md#using-with-claude-code), [Using with Codex](./connect-clients.md#using-with-codex) and [API compatibility](./compatibility.md#limitations).

## A request is blocked or limited

A `429` response names the [budget](./how-it-works.md#administering-the-gateway) that would be exceeded and when more becomes available. A `413` response means the request body is larger than the gateway's request size limit, set under **LLM Gateway** → **Settings** (100 MB by default); a change to the limit applies at once on every replica. A blocked request names the [applied guardrail](../functionality/guardrails.md#applied-guardrails) that blocked it, unless the applied guardrail has its own block message; an administrator can find it, and its outcome, on the request's entry in the LLM audit log under **Operations > Audit Logs**.

## The upstream provider rejects the request

An administrator should inspect the provider and its keys under **LLM Gateway > Providers**, the upstream credentials, requested model or deployment, and any upstream quota or rate-limit error. Check network access from Obot to the provider. Repeatedly changing the user's Obot key will not repair an invalid upstream credential.


## Verify the repair

Repeat a small request using the same client and the affected user's access. Check the returned model and the new audit outcome. A successful request under an administrator's account does not establish another user's access. For an unreachable Obot service, continue to [deployment troubleshooting](../operations/troubleshooting.md).
