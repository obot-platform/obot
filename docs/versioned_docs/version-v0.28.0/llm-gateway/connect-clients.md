---
title: Connect clients and applications
---

Every client reaches every model through the same gateway base URL, `https://<your-obot-host>/api/llm-proxy/v1`, and authenticates with an Obot API key. On the **Connect** tab of the [LLM Gateway page](./how-it-works.md#the-models-page), select a client's card to get ready-to-copy configuration for it, filled in with your Obot URL and the model you pick.

## Getting your API key {#getting-your-api-key}

Your API key for the gateway must include LLM proxy access. Print one with the CLI:

```bash
obot login --url https://obot.example.com --scope llm --print-token
```

:::note
Add `--no-expiration` if you want a key that doesn't expire.
:::

## Quick start with curl {#quick-start-with-curl}

Use the model names shown on the **LLM Gateway** page. These examples assume you have access to `openai/gpt-5.5` and `anthropic/claude-opus-4-8`.

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# OpenAI Chat Completions, with a fallback
curl https://obot.example.com/api/llm-proxy/v1/chat/completions \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "content-type: application/json" \
  -d '{"model":"openai/gpt-5.5","messages":[{"role":"user","content":"hi"}],"fallbacks":["anthropic/claude-opus-4-8"]}'

# OpenAI Responses
curl https://obot.example.com/api/llm-proxy/v1/responses \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "content-type: application/json" \
  -d '{"model":"openai/gpt-5.5","input":[{"role":"user","content":"hi"}]}'

# Anthropic Messages
curl https://obot.example.com/api/llm-proxy/v1/messages \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"anthropic/claude-opus-4-8","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}'

# List the models you can use
curl https://obot.example.com/api/llm-proxy/v1/models \
  -H "Authorization: Bearer $OBOT_API_KEY"
```

Any format works with any model. For example, send a Bedrock or Azure model to `/v1/messages` or `/v1/chat/completions` by its gateway name, such as `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`.

## Using with Claude Code {#using-with-claude-code}

Claude Code can route through the gateway and discover which models you have access to.

```bash
# 1. Point Claude Code at the gateway and authenticate with your Obot API key.
export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy"
export ANTHROPIC_AUTH_TOKEN="$(obot login --url https://obot.example.com --scope llm --print-token)"

# 2. (Optional) Pick a model. Any gateway model works, including non-Anthropic ones.
export ANTHROPIC_MODEL="anthropic/claude-sonnet-4-5"

# 3. (Optional) Discover the Claude models you have access to at startup.
#    Claude Code queries the gateway's /v1/models endpoint in Anthropic's format,
#    which lists the Claude models you can use, from any provider, by their model IDs,
#    and adds the results to the /model picker.
#    Requires Claude Code v2.1.129 or later.
export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1

# 4. Launch Claude Code.
claude
```

Inside Claude Code:

- Run **`/model`** and select an Obot gateway model. Discovered models are labeled **"From gateway"**.
- Run **`/status`** to confirm which authentication method is active.

Claude Code reaches every gateway model, including Bedrock and Azure models, through the same base URL, but its `/model` picker lists only Claude models. Pass other models by name, for example `claude --model openai/gpt-5.5`. You don't need Claude Code's Bedrock or Foundry modes.

### Claude Code with a Claude subscription {#claude-code-with-a-claude-subscription}

When an administrator turns on **Allow Claude subscriptions** on the Anthropic provider's page (**LLM Gateway** → **Providers** → **Anthropic**), users signed in to Claude Code with a Claude Pro, Max, Team or Enterprise subscription can use it through the gateway. Removing the Anthropic provider turns this off. Their requests to Claude models are billed to their subscription instead of a provider key configured in Obot. Obot still authenticates the user, enforces model access and guardrails, and records the requests in the audit log and usage reports, with their token counts but no spend, since Obot didn't pay for them.

The user keeps their Claude Code login and sends their Obot API key in the `X-Obot-Api-Key` header:

```bash
export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy"
export ANTHROPIC_CUSTOM_HEADERS="X-Obot-Api-Key: $(obot login --url https://obot.example.com --scope llm --print-token)"
# Don't set ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN; they replace the subscription login.
# Pick the model by name: see below.
export ANTHROPIC_MODEL="anthropic/claude-sonnet-4-5"
claude
```

The Connect tab's setup for this mode turns on gateway model discovery, but it hasn't been confirmed that Claude Code uses discovery with a subscription login. To be sure of the model, set `ANTHROPIC_MODEL` or pass `claude --model`.

The gateway needs the **Anthropic** provider, but it doesn't need a key. Claude Code's model names only resolve once the gateway lists the models: with an enabled key, it lists them through that key; with no keys, or only disabled ones, it lists Anthropic's current models from its model catalog, both on the **LLM Gateway** page, where profiles grant them, and in `/v1/models`, where clients discover them. Requests sent with an Obot API key instead of a subscription still need an enabled key.

Only requests from Claude Code to Anthropic models use the subscription, and the subscription token is only ever sent to Anthropic:

- It's only forwarded while the Anthropic provider sends requests straight to `https://api.anthropic.com`, with certificate checks on. If an administrator changes the provider's base URL or TLS settings, including a CA in its proxy settings, the token isn't forwarded and requests use Obot's key instead. While Claude subscriptions are allowed, a CA can't be added to the Anthropic provider's proxy settings. The environment's egress proxy (`HTTPS_PROXY`) doesn't stop the token being forwarded, since connections through it still verify Anthropic's certificate.
- A routing rule or fallback can't send a subscription request to another provider, including a custom provider built on Anthropic's API. That attempt is refused with HTTP 403.
- Requests the user sends to other models use Obot's configured keys.

Budgets don't apply to requests the subscription pays for: they aren't refused when a budget is exhausted, and they don't count toward any budget. Budgets still apply to everything else the user sends.

See the Claude Code documentation for details:

- [LLM gateway configuration](https://code.claude.com/docs/en/llm-gateway)
- [Model configuration](https://code.claude.com/docs/en/model-config) (the `/model` picker and gateway discovery)
- [Authentication](https://code.claude.com/docs/en/authentication) (`/login` and `/logout`)
- [Environment variables](https://code.claude.com/docs/en/env-vars)

## Using with Codex {#using-with-codex}

Codex uses the OpenAI Responses API, which the gateway serves for every model. Define a custom model provider in your Codex config.

1. Add the following to `~/.codex/config.toml`:

   ```toml
   model = "openai/gpt-5.5"
   model_provider = "obot"

   [model_providers.obot]
   name = "Obot LLM Gateway"
   base_url = "https://obot.example.com/api/llm-proxy/v1"
   env_key = "OBOT_API_KEY"
   wire_api = "responses"
   ```

   For a local deployment, use `base_url = "http://localhost:8080/api/llm-proxy/v1"`.

2. Set your API key and start Codex:

   ```bash
   export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
   codex        # Codex CLI
   # or
   codex app    # Codex App
   ```

Set `model` to a model name shown on the **LLM Gateway** page.

:::note
The provider ID (`obot` above) can be any name except the reserved IDs `openai`, `ollama`, and `lmstudio`. Codex's built-in web search is an OpenAI server-side tool. When you use a model that OpenAI doesn't serve, such as a Bedrock or Azure model, add `web_search = "disabled"` to the config.
:::

See the Codex documentation for details:

- [Configuration reference](https://developers.openai.com/codex/config-reference) (the `[model_providers]` keys)
- [Advanced configuration](https://developers.openai.com/codex/config-advanced) (custom providers and `wire_api`)

## Using the OpenAI and Anthropic SDKs {#using-the-openai-and-anthropic-sdks}

The OpenAI and Anthropic SDKs read their base URL and key from the environment. Point them at the gateway and use your Obot API key. Either SDK can call any gateway model.

```bash
# OpenAI SDK
export OPENAI_BASE_URL="https://obot.example.com/api/llm-proxy/v1"
export OPENAI_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# Anthropic SDK
export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy"
export ANTHROPIC_AUTH_TOKEN="$(obot login --url https://obot.example.com --scope llm --print-token)"
```

For example, with the Python SDKs:

```python
from openai import OpenAI

client = OpenAI()
r = client.chat.completions.create(model="openai/gpt-5.5", messages=[{"role": "user", "content": "hello"}])
print(r.choices[0].message.content)
```

```python
import anthropic

client = anthropic.Anthropic()
m = client.messages.create(model="anthropic/claude-opus-4-8", max_tokens=512, messages=[{"role": "user", "content": "hello"}])
# Reasoning models answer with thinking blocks before the text
print(next(b.text for b in m.content if b.type == "text"))
```

## Using with agent frameworks {#using-with-agent-frameworks}

Agent frameworks reach the gateway through its OpenAI-compatible endpoint, so they can call any gateway model. Replace `openai/gpt-5.5` with a model name shown on the **LLM Gateway** page.

### Google Agent Development Kit (ADK) {#using-with-google-adk}

ADK reaches OpenAI-compatible endpoints through its LiteLLM model wrapper. Install ADK and LiteLLM, and create an agent package:

```bash
pip install google-adk litellm
mkdir -p obot_agent && echo "from . import agent" > obot_agent/__init__.py
```

Define the agent in `obot_agent/agent.py`:

```python
import os

from google.adk.agents import Agent
from google.adk.models.lite_llm import LiteLlm

root_agent = Agent(
    name="assistant",
    # LiteLLM's openai/ prefix, then the gateway model name
    model=LiteLlm(
        model="openai/openai/gpt-5.5",
        api_base="https://obot.example.com/api/llm-proxy/v1",
        api_key=os.environ["OBOT_API_KEY"],
    ),
    instruction="You are a helpful assistant.",
)
```

LiteLLM removes only the first `openai/` and sends the rest, such as `anthropic/claude-opus-4-8`, to the gateway as the model name. Set your API key and run the agent:

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
adk run obot_agent
```

See [ADK's LiteLLM models](https://adk.dev/agents/models/litellm/) and [LiteLLM's OpenAI-compatible endpoints](https://docs.litellm.ai/docs/providers/openai_compatible).

### Vercel AI SDK {#using-with-the-vercel-ai-sdk}

Use the AI SDK's OpenAI-compatible provider, which calls the gateway's Chat Completions endpoint:

```bash
npm install ai @ai-sdk/openai-compatible
```

Save the following as `index.mts`:

```ts
import { createOpenAICompatible } from '@ai-sdk/openai-compatible';
import { generateText } from 'ai';

const obot = createOpenAICompatible({
  name: 'obot',
  baseURL: 'https://obot.example.com/api/llm-proxy/v1',
  apiKey: process.env.OBOT_API_KEY,
});

const { text } = await generateText({
  model: obot('openai/gpt-5.5'),
  prompt: 'hello',
});
console.log(text);
```

Set your API key and run it:

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
npx tsx index.mts
```

See [the AI SDK's OpenAI-compatible providers](https://ai-sdk.dev/providers/openai-compatible-providers).

## Using with open-source agents {#using-with-open-source-agents}

These agents use the gateway's OpenAI-compatible endpoint, so they can call any gateway model. Replace `openai/gpt-5.5` with a model name shown on the **LLM Gateway** page.

### OpenCode {#using-with-opencode}

Add a custom provider to `~/.config/opencode/opencode.json` (or `opencode.json` in your project):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "obot/openai/gpt-5.5",
  "provider": {
    "obot": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Obot LLM Gateway",
      "options": {
        "baseURL": "https://obot.example.com/api/llm-proxy/v1",
        "apiKey": "{env:OBOT_API_KEY}"
      },
      "models": {
        "openai/gpt-5.5": {
          "name": "openai/gpt-5.5"
        }
      }
    }
  }
}
```

Then set your API key and start OpenCode:

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
opencode
```

Add an entry under `models` for each gateway model you want in OpenCode's model list. See [OpenCode providers](https://opencode.ai/docs/providers/) and [OpenCode config](https://opencode.ai/docs/config/).

### Goose {#using-with-goose}

Goose's OpenAI provider takes the host and the request path separately:

```bash
export GOOSE_PROVIDER="openai"
export GOOSE_MODEL="openai/gpt-5.5"
export OPENAI_HOST="https://obot.example.com"
export OPENAI_BASE_PATH="api/llm-proxy/v1/chat/completions"
export OPENAI_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
goose session
```

See [Goose providers](https://goose-docs.ai/docs/getting-started/providers).

### Cline {#using-with-cline}

1. In Cline's settings (the gear icon), set **API Provider** to **OpenAI Compatible**.
2. Set **Base URL** to `https://obot.example.com/api/llm-proxy/v1`.
3. Set **API Key** to the key printed by `obot login --url https://obot.example.com --scope llm --print-token`.
4. Set **Model ID** to a gateway model name, such as `openai/gpt-5.5`.

See [Cline's OpenAI Compatible provider](https://docs.cline.bot/provider-config/openai-compatible).

### Open WebUI {#using-with-open-webui}

1. As an Open WebUI admin, open **Settings** > **Admin** > **Connections** and, under **Manage OpenAI API Connections**, select **Add Connection**.
2. Set **URL** to `https://obot.example.com/api/llm-proxy/v1`.
3. Set **API Key** to the key printed by `obot login --url https://obot.example.com --scope llm --print-token`.
4. Save, then choose a gateway model from the model menu in a new chat.

Or, for a new Open WebUI install, set the connection when starting it with Docker:

```bash
docker run -d -p 3000:8080 \
  -e OPENAI_API_BASE_URL="https://obot.example.com/api/llm-proxy/v1" \
  -e OPENAI_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)" \
  -v open-webui:/app/backend/data --name open-webui --restart always \
  ghcr.io/open-webui/open-webui:main
```

For an Obot on the same machine, such as `http://localhost:8080`, use `http://host.docker.internal:8080/api/llm-proxy/v1` instead, and on Linux add `--add-host=host.docker.internal:host-gateway`: inside the container, `localhost` is Open WebUI itself. The Connect tab does this for you, in both the steps and the Docker command, when Obot's URL is `localhost`. If Open WebUI runs on the same machine without Docker, keep the `localhost` URL.

Every Open WebUI user's requests go through the gateway with this one key, so Obot attributes them to the key's owner. See [Open WebUI's OpenAI-compatible setup](https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/).

:::note
Cline and Open WebUI store the key you give them. Add `--no-expiration` to `obot login` if you want a key that doesn't expire.
:::

For supported endpoints and known limits, see [API compatibility](./compatibility.md). If a request fails, see [Troubleshooting](./troubleshooting.md).
