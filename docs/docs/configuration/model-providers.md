---
title: "Configure providers and models"
---

# Configure providers and models {#model-providers}

Obot reaches models through its built-in [LLM gateway](../llm-gateway/how-it-works.md). Administrators add the upstream providers it serves, and their keys, under **LLM Gateway** → **Providers**. Every provider is available in the default Obot edition without Community registration or an Enterprise license. You still need credentials and access to the upstream model service.

## Adding providers and keys {#adding-providers-and-keys}

The gateway serves models from OpenAI, Anthropic, Amazon Bedrock, Google Vertex AI and Gemini, Azure, Mistral, Groq, xAI, DeepSeek, OpenRouter, Cohere, vLLM, Ollama and more. On the **Providers** tab of the **LLM Gateway** page:

- **Add a provider and its keys.** Several keys on one provider are load-balanced by weight, and a key can be limited to specific models.
- **Keep retries on for failover.** A request moves to another of the provider's keys only when it is retried. New providers retry twice, with backoff from 500 ms to 5 s.
- **Tune the connection.** Set base URLs (for self-hosted or proxied endpoints), timeouts, retries with backoff, concurrency and extra headers.
- **Add custom providers.** A custom provider points at any OpenAI-, Anthropic-, Gemini- or Cohere-compatible endpoint.
- **Set the default models.** **Set Default Models** chooses the models behind the aliases Obot's own features use. See [Default models](../llm-gateway/how-it-works.md#default-models).

Models from gateway providers are listed on the **LLM Gateway** page under the *LLM Gateway* provider. Adding a provider does not make its models available to every user: grant them with [Model Access Policies](../functionality/model-access-policies.md).

Every LLM gateway provider is available in every edition, without registration or a license key. You still need credentials and access to the upstream model service.

## Provider notes {#provider-notes}

### OpenAI and Anthropic keys at startup {#openai-and-anthropic-keys-at-startup}

When the server's `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` environment variable is set, Obot configures the gateway's `openai` or `anthropic` provider with that key at startup, and updates the key when the value changes. See [Server Configuration](./server-configuration.md).

### Amazon Bedrock {#amazon-bedrock}

Add the **AWS Bedrock** provider with a Bedrock API key or an access key ID and secret access key, and the region.

Anthropic models on Bedrock require you to submit Anthropic's use case details form in your AWS account first. Until you do, requests can fail with `Model use case details have not been submitted for this account`. See the [AWS model access documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/model-access.html). Model availability depends on region; see the [AWS Bedrock Anthropic model cards](https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards-anthropic.html).

### Azure {#azure}

Add the **Azure OpenAI / AI Foundry** provider with the resource endpoint and either an API key or the tenant ID, client ID and client secret of a Microsoft Entra service principal. Assign the service principal **Cognitive Services User** or **Foundry User** (formerly Azure AI User) on the Foundry resource. **Cognitive Services OpenAI User** can allow OpenAI requests while Anthropic requests fail with `Principal does not have access to API/Operation`. See [Configure keyless authentication with Microsoft Entra ID](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/how-to/configure-entra-id).

A key with neither uses Obot's own managed or workload identity. Its endpoint must then be an Azure OpenAI or AI Services host (`https://<resource>.openai.azure.com`, `.cognitiveservices.azure.com` or `.services.ai.azure.com`), and it can't request other scopes.

Certificate checks can't be turned off, and a custom CA can't be added, on Azure, Vertex AI and Bedrock providers, since they may use Obot's own cloud identity.

### Ollama, vLLM and other self-hosted models {#ollama-vllm-and-other-self-hosted-models}

Add the built-in **Ollama** or **vLLM** provider with the server URL; they don't need a key. For other OpenAI-compatible services, such as LiteLLM, add a custom OpenAI-compatible provider with its base URL. A base URL ending in `/v1`, as such servers usually document it, is saved without it, since the gateway adds `/v1` to every path itself.

Many OpenAI-compatible servers serve only Chat Completions, not the OpenAI Responses API. Leave **Serves the OpenAI Responses API** off for them when adding the custom provider: the gateway then serves Responses API and Anthropic Messages requests (from Codex or Claude Code, for example) to it through Chat Completions. Turn it on only for a server that serves `/v1/responses`. Through the API, the same choice is the provider's `custom_provider_config.allowed_requests`: list the request types the server serves (`list_models`, `chat_completion`, `chat_completion_stream`, `embedding` and so on), leaving out `responses` and `responses_stream`.

When Obot runs in Docker and Ollama runs on the host, start Ollama with `OLLAMA_HOST=0.0.0.0` and use `http://host.docker.internal:11434` as the server URL. On Linux, also start the Obot container with `--add-host=host.docker.internal:host-gateway`.

## Server configuration {#server-configuration}

The gateway is always on. These settings control it:

| Flag / environment variable | Description |
| --- | --- |
| `--llm-gateway-data-dir` / `OBOT_SERVER_LLM_GATEWAY_DATA_DIR` | Where the gateway keeps its configuration (default `$XDG_DATA_HOME/obot/llm-gateway`). |
| `--llm-gateway-postgres-dsn` / `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN` | Postgres database for gateway configuration. Must be a different database from Obot's. When Obot itself uses Postgres and this is unset, Obot creates a `<database>_llm_gateway` database on the same server, so every replica shares the gateway's state. If Obot's database user can't create databases, create it yourself or set this; otherwise the gateway doesn't start, rather than keep separate state on each replica. When the configuration changes, every replica reloads it within seconds; requests already in flight finish first. |
| `--llm-gateway-encryption-key` / `OBOT_SERVER_LLM_GATEWAY_ENCRYPTION_KEY` | Key that encrypts provider credentials at rest. When unset, a key is generated: with Postgres it is stored in Obot's database (encrypted with Obot's own encryption configuration, if any) and shared by every replica; with SQLite it is stored in the data dir. |
| `--llm-gateway-log-level` / `OBOT_SERVER_LLM_GATEWAY_LOG_LEVEL` | Log level for the embedded gateway (default `warn`). |
| `--llm-gateway-model-data-url` / `OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL` | Where the gateway refreshes model prices and capabilities from, every 6 hours (default `https://upgrade-server.obot.ai/v1/model-data`). Obot's published images and release binaries include a copy of this data, so the gateway works without reaching this URL; set `off` to use only that copy, for example in air-gapped installs. See [model pricing](../llm-gateway/audit.md#model-pricing). |
| `--daily-user-input-token-limit` / `--daily-user-output-token-limit` | Seed the *Daily token limit* budget, with administrators exempt, on every profile that exists when the gateway first starts (the *Default Policy* on a new installation). After that, edit or remove each profile's copy on its page; changing these settings has no effect. |
| `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` | Obot routes gateway providers through the environment's egress proxy, except endpoints `NO_PROXY` excludes or that are on localhost. A proxy set on a provider explicitly takes precedence. |

## Related topics {#related-topics}

- [LLM Gateway overview](../llm-gateway/how-it-works.md) — the LLM Gateway page, default models, routing rules and fallbacks
- [Model Access Policies](../functionality/model-access-policies.md) — grant users access to specific models
- [Upgrading from model providers](../llm-gateway/upgrade.md) — how earlier model provider configuration moves to the gateway
