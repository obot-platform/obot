---
title: Guardrails
---

## Overview

Guardrails check content flowing through Obot and act on what they find. They apply to:

- **LLM gateway traffic**: requests to models (input) and model responses (output), including streamed responses and tool calls the model makes. Guardrails applied in [profiles](./model-access-policies.md) check this traffic.
- **MCP traffic**: tool calls and their results, resource contents and prompts, for servers reached through Obot's MCP gateway. [Guardrail filters](./filters.md#guardrail-filters), configured under **MCP Servers > Filters**, check this traffic.

A guardrail can **block** the request or response, **redact** the offending text and let the rest through, or only **log** what it found.

Guardrails are made of:

- **Guardrails** each configure one check, such as secrets detection, a PII service or an LLM judge policy, with its settings and credentials. Guardrail policies and MCP guardrail filters share them.
- **Applied guardrails** (guardrail policies in the API) decide which LLM gateway traffic is checked, by which guardrails, for whom, and what happens when a guardrail finds something. Each belongs to a profile.
- **Guardrail filters** do the same for MCP traffic. They are MCP filters, so they use the same server targeting and selectors as other filters and run in the same filter chain. See [Guardrail filters](./filters.md#guardrail-filters).

Where to find them:

| What | Where |
| --- | --- |
| Guardrails | The **Guardrails** tab of **AI Resources > LLM Gateway**. Guardrails are shared, so a change applies to model and MCP traffic alike. Its **Used by** column shows the applied guardrails that use each guardrail, each with the profile it's applied in, and the guardrail filters that use it. |
| Applied guardrails for model traffic | **AI Resources > LLM Gateway > Profiles**, in each profile's **Applied Guardrails** section |
| Guardrail filters for MCP traffic | **AI Resources > MCP Servers > Filters** |
| What guardrails did | **Operations > Audit Logs**: the LLM audit log for model traffic, and the MCP audit log for guardrail filters |

## Guardrail types

When you add a guardrail, its types are grouped as **Built-in** checks that Obot runs itself and **3rd Party Integrations** with external services. You can add more than one guardrail of a type, for example with different settings; a 3rd party integration that's already configured is marked **Configured**.

| Type | What it detects | Can redact |
| --- | --- | --- |
| **Secrets Detection** (built-in) | API keys and tokens for AWS, GitHub, GitLab, Slack, Stripe, Google, OpenAI, Anthropic, Azure, Twilio, SendGrid, npm, Hugging Face and Obot; private keys, JWTs, database URLs; optionally generic secrets: high-entropy values assigned to names like `api_key`, `access_key`, `client_secret`, `token` or `password`, including prefixed names such as `AWS_SECRET_ACCESS_KEY`. camelCase names such as `clientSecret` and subscripts such as `env["API_KEY"]` count only when assigned a quoted value, so code that assigns another variable or a call isn't flagged. | Yes |
| **Custom Regex** (built-in) | Your RE2 patterns, plus ready-made patterns for email addresses, phone numbers (North American, and international numbers written with their `+` country code, but not a `+` before a date), US SSNs, credit cards (checksum-verified), IPv4 and IPv6 addresses (dotted runs of more than four numbers, such as versions, and IPv6-looking names without a digit, such as `dead::beef`, aren't matched), and IBANs, with or without spaces (checksum-verified). | Yes |
| **LLM Judge Policy** (built-in) | Anything you can describe in plain language, e.g. "Do not help book travel above economy class". On model traffic it judges the latest user message (input) and the model's tool calls (output), not the model's text replies. Screened by the `Chat - Fast` default model, with a second review by the `Chat` default model before blocking. This is what [message policies](./ai-judge-policies.md) became. | No |
| **Microsoft Presidio** | Personal data, using your self-hosted Presidio Analyzer. By default: email addresses, phone numbers, credit cards, US SSNs, ITINs, passports, driver's licenses and bank numbers, IBANs and IP addresses. | Yes |
| **Azure AI Language PII** | Personal and health data. | Yes |
| **Azure AI Content Safety** | Hate, sexual, violence and self-harm content above a severity threshold; optionally prompt injection with Prompt Shields. | No |
| **AWS Bedrock Guardrails** | Whatever the Bedrock guardrail is configured for: denied topics, content and word filters, sensitive information and contextual grounding. | Anonymized content |
| **Google Cloud Model Armor** | Responsible AI filters, prompt injection and jailbreaks, malicious URLs, CSAM and Sensitive Data Protection, per the Model Armor template. | Sensitive data |
| **Lakera Guard (Check Point)** | Prompt attacks, PII, content violations, unknown links and custom detectors. With a **Project ID**, that project's policy decides what is flagged. Without one, Lakera's default policy screens the content and only results at or above **Minimum confidence** (by default `l2_very_likely`) count, since the default policy also flags some ordinary prompts. | PII, profanity, custom regex |
| **Patronus AI** | Results of Patronus evaluators, such as toxicity and PII detectors or LLM judges with your criteria. | No |
| **CrowdStrike Falcon AIDR** | Malicious prompts, PII, secrets, malicious entities, topics and more, per the collector's policy. | Content AI Guard redacts |

Credentials such as API keys are stored encrypted and are never shown again after they are saved. Leave a credential blank when editing a guardrail to keep the current value. Changing where a guardrail's credentials are sent (its endpoint, base URL, region, analyzer URL or location) requires entering them again, so stored credentials can't be redirected to another server. A guardrail service may be on a private network, such as a self-hosted Presidio at `http://presidio-analyzer:3000`, but not on Obot's own host (`localhost`, `127.0.0.0/8`, `::1`), a link-local address (`169.254.0.0/16`, `fe80::/10`) or a cloud metadata address such as `169.254.169.254` or AWS's `fd00:ec2::254`. Host names are checked when a guardrail is saved and again on every connection. This differs from remote MCP servers, whose URLs may not resolve to private addresses unless an administrator sets `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP=false` (see [Remote server](../mcp-gateway/register-remote.md)): remote MCP servers can be added by users with the Power User role, while guardrails are configured only by administrators and their services often run next to Obot. A Google Cloud Model Armor guardrail without a service account key uses Obot's own application default credentials, so it only sends requests to Model Armor's own endpoints (`modelarmor.googleapis.com`, regional `modelarmor.<location>.rep.googleapis.com`, or a Private Service Connect `*.p.googleapis.com` endpoint); any other endpoint requires a service account key.

For a Presidio guardrail, **Load all types from the service** lists every entity type your Analyzer supports, including its custom recognizers. Types such as `PERSON`, `LOCATION`, `DATE_TIME` and `URL` aren't checked by default, because they match ordinary text, like the names of public figures, cities, "Friday" and links.

Use **Test** on a saved guardrail to run it against sample text and see what it finds and how the text would be redacted.

A guardrail can't be deleted while an applied guardrail or a guardrail filter uses it.

## Applied guardrails

Guardrails are applied to LLM gateway traffic in [profiles](./model-access-policies.md): open a profile and click **Add Guardrail** in its **Applied Guardrails** section, or click one to change it. Applied guardrails are saved with the profile. The API calls them guardrail policies. They apply to LLM gateway traffic only. To check MCP traffic, create a [guardrail filter](./filters.md#guardrail-filters). Each applied guardrail has:

- **Apply to**: input, output or both.
- **Guardrails**: one or more guardrails to run.
- **Action**:
  - **Block** rejects the request, or ends the response, with a message naming the policy and what was found. You can replace that message with your own.
  - **Redact** replaces what was found with a label such as `[REDACTED:EMAIL]` and lets the rest through. Some guardrails can only tell that content is a problem, not where it is. A redacting policy blocks when one of those finds something.
  - **Log** records the finding in the request's LLM audit log entry and lets the request through unchanged.
- **Profile**: the profile it's applied in. It applies to that profile's users and groups. The applied guardrails of every profile a user is in apply to them.
- **Condition** (optional): a [CEL](https://github.com/google/cel-spec) expression that must be true for the policy to apply.
  - It can use `model` (for example `"openai/gpt-4.1"`), `provider`, `params` (currently only `params.max_tokens`, when the request sets it; test with `"max_tokens" in params`), `headers`, `user.id` and `user.groups`.
  - `user.groups` holds the IDs of the user's auth provider groups, the same IDs that group subjects use. It doesn't hold group names or Obot roles.
  - For example: `model.startsWith("openai/") && !("<contractors group ID>" in user.groups)`.
  - Reading a header that isn't there is an error, so test for it first: `"x-team" in headers && headers["x-team"] == "sales"`. A condition that fails to evaluate is logged by the Obot server (at most once a minute per policy); the policy is then skipped, or blocks the request if it fails closed.
- **Sampling rate**: the percentage of matching traffic to check, for expensive guardrails.
- **Input scope**: whether input checks look at the whole conversation or only what was added since the model last replied. Redacting policies always check the whole conversation, so nothing unredacted is sent.
- **Fail closed**: whether to block when a guardrail or the policy's condition can't be evaluated, for example because an external service or the LLM judge's model is down. By default the policy is skipped, and a guardrail's error is recorded on the request's LLM audit log entry.

Guardrail policies run in the order they were created. A block stops evaluation. Each redaction is applied before the next policy runs.

## How checks are applied

### LLM gateway

Input policies run before anything else sees the request, so the provider never sees content before it is redacted. Output policies run after the response arrives and before it is returned. Token counting requests (`/v1/messages/count_tokens`, `/v1/responses/input_tokens`) carry the whole prompt to the provider and are checked as input. Obot's own [LLM audit log](../llm-gateway/audit.md) records requests as clients sent them, before redaction.

Checks cover message text, tool calls and tool results, and also reasoning (thinking) and refusal text. Redacting signed reasoning, such as Anthropic extended thinking, invalidates its signature, so the provider may reject a later request that sends that reasoning back.

Some clients, notably Claude Code, normally have their requests forwarded to Anthropic as sent. When any guardrail applies to a request, Obot instead sends the request it checked, and returns the response it checked, so nothing bypasses the guardrail policies.

For **streamed responses**, Obot holds the stream while a policy that can block or redact applies. It checks the complete response, then delivers it at once: redacted if needed, or replaced by an error if blocked. A secret split across stream chunks is still found. Log-only policies don't hold the stream unless they fail closed. If a stream can't be held, a finding that would block or redact ends the stream and is recorded as an error, since the content has already been delivered.

A blocked request gets HTTP 400 with the block message. In the OpenAI Chat Completions and Responses formats the error's type is `guardrail_intervention`. In the Anthropic Messages format it is `invalid_request_error`, since Anthropic clients such as Claude Code only expect Anthropic's own error types.

### MCP gateway

MCP traffic is checked by [guardrail filters](./filters.md#guardrail-filters), which run in-process in the MCP gateway's filter chain, in order with any other filters. See that page for what is checked and how outcomes are audited.

## Where outcomes are recorded {#events}

What guardrail policies did to a request is recorded on its [LLM audit log](../llm-gateway/audit.md#audit-logs-and-usage-whats-logged-1) entry: each policy that found something or failed, its guardrail, the stage, the outcome (blocked, redacted, logged or error), the categories found, and, for an error, why the guardrail couldn't be evaluated. It never includes the offending content.

What guardrail filters did to MCP traffic is recorded in the MCP audit log, as described under [Guardrail filters](./filters.md#guardrail-filters).

## Message policies

Message policies are now LLM Judge guardrails. When Obot starts, each existing message policy is converted once into an LLM Judge guardrail and a blocking guardrail policy. The policy belongs to the profile for everyone if the policy applied to everyone, or to a new profile, named `<policy name> (message policy)`, with the policy's users and groups. A policy that applied to no one gets a profile with no users or groups, so its guardrail policy applies to no one. Add users or groups to that profile before you enable the policy, since a profile can't be saved without them.

- Policies on **user messages** become input policies.
- Policies on **tool calls** become output policies.
- Policies on **both** become policies for input and output.

The guardrail policies start disabled. To enable one, open its profile, click the applied guardrail, turn on **Enabled** at the top of the dialog, click **Apply**, then **Save** the profile. They fail closed, as message policies did: when the judge's model is unavailable, the request is blocked.

The original policies are kept but no longer enforced. The message policy API was removed.

## API

Administrators can manage guardrails and guardrail policies with these endpoints, under `/api/llm-gateway`:

| Endpoint | Purpose |
| --- | --- |
| `GET, POST /guardrails`, `GET, PUT, DELETE /guardrails/{id}` | Guardrails (credentials are write-only) |
| `POST /guardrails/{id}/test` | Run a guardrail against sample text |
| `GET /guardrails/providers` | Guardrail types and their settings |
| `POST /guardrails/providers/{type}/fields/{field}/options` | List a guardrail field's choices from the service its settings point at, such as the entity types a Presidio Analyzer supports. Send the guardrail's `config`, and `guardrailID` to use a saved guardrail's credentials. |
| `POST /guardrails/validate-condition` | Check a CEL condition |
| `GET, POST /guardrail-policies`, `GET, PUT, DELETE /guardrail-policies/{id}` | Guardrail policies (`target` must be `llm`, `profileID` names the profile and `guardrailIDs` the guardrails). `enabled` defaults to false, so set `"enabled": true`. `?profileID=` lists one profile's. |

Guardrail filters are managed with the filter API, `/api/mcp-webhook-validations`. A guardrail filter has a `guardrail` object in place of a `url`, `mcpServerManifest` or `systemMCPServerCatalogEntryID`:

```json
{
  "name": "No secrets in tool calls",
  "resources": [{ "type": "mcpCatalog", "id": "default" }],
  "selectors": [{ "method": "tools/call" }],
  "guardrail": {
    "guardrailIDs": ["<guardrail id>"],
    "action": "redact",
    "direction": "both",
    "failClosed": false,
    "samplingRate": 100,
    "subjects": [{ "type": "group", "id": "<group id>" }],
    "condition": "mcp_tool != \"search\"",
    "blockMessage": ""
  }
}
```

`action` is `block` or `redact`; guardrail filters can't log only. `direction` is `request`, `response` or `both` (the default). `samplingRate` defaults to 100. `subjects` are `user`, `group` or `everyone`; none means everyone. `allowedToMutate` is set from the action (true for `redact`) and doesn't otherwise apply to guardrail filters.

## Related topics

- [LLM Gateway](../llm-gateway/how-it-works.md)
- [MCP Servers](./mcp-servers.md)
- [Filters](./filters.md)
- [LLM audit logs](../llm-gateway/audit.md)
- [MCP audit logs](../security/audit-data.md)
