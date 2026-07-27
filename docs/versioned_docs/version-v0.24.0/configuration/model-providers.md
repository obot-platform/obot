# Model Providers

The Model Providers page allows administrators to configure and manage various AI model providers. This guide will walk you through the setup process and explain the available options.

### Configuring Model Providers

Obot supports a variety of model providers, including:

**Community**
- OpenAI
- Anthropic
- [Generic Responses Compatible Provider](#generic-responses-compatible-provider)

**Enterprise**
- [Azure OpenAI / Microsoft Foundry](#azure-enterprise-only)
- [Amazon Bedrock](#amazon-bedrock-enterprise-only)
- Google Vertex (Gemini models)

The UI will indicate whether each provider has been configured. If a provider is configured you will have the ability to modify or deconfigure it.

:::note
Our Enterprise release adds support for additional Enterprise-grade model providers. [See here](/enterprise/overview/) for more details.
:::

#### Configuring and enabling a provider

To configure a provider:

1. Click its "Configure" button
2. Enter the required information, such as API keys or endpoints
3. Save the configuration to apply the settings

Upon saving the configuration, the platform will validate your configuration to ensure it can connect to the model provider. You can configure multiple model providers, which will allow you to pick the right provider and model for each use case.

### Viewing and managing models

Once a provider is configured, you can view and manage the models it offers. You can set the usage type for each model, which determines how the models are utilized within the application:

| Usage Type | Description | Application |
|------------|-------------|-------------|
| **Language Model** | Used to drive text generation and tool calls | Used in agents and tasks; can be set as an agent's primary model |
| **Text Embedding** | Converts text into numerical vectors | Used in the knowledge tool for RAG functionality |
| **Image Generation** | Creates images from textual descriptions | Used by image generation tools |
| **Vision** | Analyzes and processes visual data | Used by the image vision tool |
| **Other** | Default if no specific usage is selected | Available for all purposes |

You can also activate or deactivate specific models, controlling their availability to users.

### Setting Default Models

The "Set Default Models" feature allows you to configure default models for various tasks. Choose default models for the following categories:

- **Language Model (Chat)** - Primary conversational model
- **Language Model (Chat - Fast)** - Optimized for quick responses

These defaults determine which specific model is used when a [Model Access Policy](/functionality/model-access-policies/) grants access to a default model alias (such as "Language Model (Chat)"). When you change a default here, any user with access to that alias automatically gains access to the new model.

After selecting the desired defaults, click "Save Changes" to confirm your configurations.

:::note
Setting a default model here does not automatically grant users access to it. Users must be included in a Model Access Policy that grants access to the corresponding alias. See [Model Access Policies](/functionality/model-access-policies/) for details.
:::

### Instructions for configuring specific providers

#### Azure (Enterprise only)

Obot supports two Azure providers, each with a different authentication method. These are compatible with both Azure OpenAI deployments and Foundry deployments.

##### API Key Authentication

Use the **Azure** provider for API key-based authentication.

In the Azure portal, find your API key and endpoint URL after setting up at least one deployment — both are required.

You must also specify deployment names as a comma-separated list using `deployment[:usage[:dialect]]`. Usage defaults to `llm`. Dialect defaults to `openai`; specify `anthropic` for Claude deployments. For example: `my-gpt-deployment,my-claude-deployment:llm:anthropic`.

##### Microsoft Entra ID Authentication

Use the **Azure (Entra ID)** provider for service principal authentication via Microsoft Entra ID. Deployments are discovered automatically from the Azure Management API.

###### 1. Create a service principal

```bash
az ad sp create-for-rbac --name "<sp-name>" \
  --role "Cognitive Services User" \
  --scopes /subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.CognitiveServices/accounts/<resource-name>
```

This outputs the `appId` (Client ID), `password` (Client Secret), and `tenant` (Tenant ID) needed below.

###### 2. Find your resource details

```bash
az cognitiveservices account show \
  --name <resource-name> \
  --resource-group <resource-group> \
  --query "{endpoint:properties.endpoint, id:id}"
```

###### 3. Configure the provider

Obot requires:
- **Azure Endpoint** — your Azure OpenAI endpoint URL (`https://<resource_name>.openai.azure.com`)
- **Client ID** — the Entra app's application (client) ID
- **Client Secret** — the Entra app's client secret
- **Tenant ID** — the Entra app's tenant ID
- **Subscription ID** — the Azure subscription ID containing the Cognitive Services account
- **Resource Group** — the resource group containing the Cognitive Services account
- **Resource Name** — the Cognitive Services resource name

The service principal requires the **Cognitive Services User** role on the specific Foundry resource for inference. **Foundry User** (formerly Azure AI User) also includes the permissions required to invoke Claude models. The narrower **Cognitive Services OpenAI User** role can allow OpenAI requests while Anthropic requests fail with `Principal does not have access to API/Operation`.

Deployments are discovered automatically. Obot uses the Azure deployment name as the target model and derives the request dialect from the deployment's model metadata.

Official references:

- [Configure keyless authentication with Microsoft Entra ID](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/how-to/configure-entra-id) — required inference role and resource scope
- [Claude in Microsoft Foundry](https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry) — Claude authentication and RBAC requirements
- [Claude Code on Microsoft Foundry](https://code.claude.com/docs/en/microsoft-foundry#azure-rbac-configuration) — Claude Code RBAC requirements
- [Assign Azure roles using the Azure portal](https://learn.microsoft.com/en-us/azure/role-based-access-control/role-assignments-portal) — portal instructions and role-assignment permissions

#### Amazon Bedrock (Enterprise only)

Obot supports two Amazon Bedrock providers, each with a different authentication method.

:::note
Both Bedrock providers use _AWS Bedrock Inference Profiles_ rather than direct on-demand model access. Inference profiles are resources that route model invocation requests and enable cost tracking — AWS provides system-defined cross-region inference profiles by default for supported models, so no manual setup is typically required. Only models with an available inference profile will appear in Obot. See the [AWS documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles-use.html) for more details.
:::

:::warning
**First-time Anthropic model use requires explicit access.** When using Anthropic models through Bedrock on an AWS account that has not previously been granted access, requests may succeed initially and then begin failing with the following error:

> Model use case details have not been submitted for this account. Fill out the Anthropic use case details form before using the model. If you have already filled out the form, try again in 15 minutes.

This is expected AWS behavior — Anthropic models on Bedrock require you to submit a use case details form before they can be used. See the [AWS model access documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/model-access.html) to request and enable access for the Anthropic models you intend to use.
:::

##### Static Credentials

Use the **Amazon Bedrock (Static Credentials)** provider to authenticate with long-lived AWS credentials.

Obot requires:
- **AWS Access Key ID** — your IAM user's access key
- **AWS Secret Access Key** — your IAM user's secret key
- **AWS Region** — the region where your inference profiles are configured (e.g. `us-east-1`)
- **AWS Session Token** (optional) — required when using temporary security credentials from AWS STS (e.g. when assuming an IAM role or using federated access). Not needed for long-lived IAM user credentials.

##### API Key

Use the **Amazon Bedrock (API Key)** provider to authenticate with a Bedrock API key.

Obot requires:
- **API Key** — your Bedrock API key
- **AWS Region** — the region where your inference profiles are configured (e.g. `us-east-1`)

#### Generic Responses Compatible Provider

Use **Generic Responses Compatible Provider** to connect Obot to any Responses-compatible API.

This provider supports:
- A provider-level **Base URL** and optional **API Key**

##### Common examples

- **Ollama**: `http://127.0.0.1:11434/v1`
- **LiteLLM**: `http://<litellm-host>:4000/v1`

##### Using Ollama

[Ollama](https://ollama.ai/) allows you to run models locally. When Obot runs in Docker, make Ollama reachable from the container:

1. **Expose Ollama to the network**
   Set `OLLAMA_HOST=0.0.0.0` before starting Ollama.

2. **Set Base URL in Obot**
   Use one of:
   - `http://host.docker.internal:11434/v1` (recommended for Docker Desktop)
   - `http://<your-host-ip>:11434/v1`

For Linux Docker, add `--add-host=host.docker.internal:host-gateway` (or use another host-networking approach) so `host.docker.internal` resolves inside the Obot container.

See [Ollama's FAQ](https://docs.ollama.com/faq) for platform-specific details.

##### LiteLLM config example

If you use LiteLLM, Obot works with LiteLLM's wildcard model list:

```yaml
model_list:
  - model_name: openai/*
    litellm_params:
      model: openai/*
      api_key: os.environ/OPENAI_API_KEY
```

In Obot, models discovered through this provider are currently treated as **Language Model (LLM chat)** models by default.

If your upstream includes non-chat-capable models, use more specific model mappings instead of a broad wildcard, or avoid using these models with Obot.
