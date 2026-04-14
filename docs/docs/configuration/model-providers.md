# Model Providers

The Model Providers page allows administrators to configure and manage various AI model providers. This guide will walk you through the setup process and explain the available options.

### Configuring Model Providers

Obot supports a variety of model providers, including:

**Community**
- OpenAI
- Anthropic
- xAI
- [Ollama](#ollama)
- Voyage AI
- Groq
- vLLM
- DeepSeek
- Google

**Enterprise**
- [Azure OpenAI / Microsoft Foundry](#azure-openai-enterprise-only)
- Amazon Bedrock
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
- **Text Embedding (Knowledge)** - Used for knowledge base operations
- **Image Generation** - For creating images
- **Vision** - For image analysis and processing

These defaults determine which specific model is used when a [Model Access Policy](../../functionality/model-access-policies/) grants access to a default model alias (such as "Language Model (Chat)"). When you change a default here, any user with access to that alias automatically gains access to the new model.

After selecting the desired defaults, click "Save Changes" to confirm your configurations.

:::note
Setting a default model here does not automatically grant users access to it. Users must be included in a Model Access Policy that grants access to the corresponding alias. See [Model Access Policies](../../functionality/model-access-policies/) for details.
:::

### Instructions for configuring specific providers

#### Azure (Enterprise only)

Obot supports two Azure providers, each with a different authentication method.

##### API Key Authentication

Use the **Azure** provider for API key-based authentication.

In the Azure portal, find your API key and endpoint URL after setting up at least one deployment — both are required.

You must also specify deployment names. The format is a comma-separated list of deployment names, optionally as `model:deployment` pairs (e.g. `gpt-4o,gpt-4o-mini` or `gpt-4o:my-gpt4o,gpt-4o-mini:my-mini`).

You can also optionally specify the API version (defaults to `2025-01-01-preview`).

##### Microsoft Entra ID Authentication

Use the **Azure (Entra ID)** provider for service principal authentication via Microsoft Entra ID.

Obot requires:
- **Azure Endpoint** — your Azure service endpoint URL
- **Client ID** — the Entra app's application (client) ID
- **Client Secret** — the Entra app's client secret
- **Tenant ID** — the Entra app's tenant ID

Optionally, you can specify deployment names (same format as above). If omitted, the provider will attempt to discover deployments automatically. You can also optionally specify the API version (defaults to `2025-01-01-preview`).

After creating your Entra app registration, go to your Azure resource in the Azure portal and add a role assignment for the app registration as a service principal with the `Cognitive Services OpenAI User` role.

See the [Microsoft docs](https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/role-based-access-control?view=foundry-classic#add-role-assignment-to-an-azure-openai-resource) for more details.

#### Ollama

[Ollama](https://ollama.ai/) allows you to run LLMs locally. Two configuration steps are required to use it with Obot:

1. **Expose Ollama to the network** - By default, Ollama only binds to `127.0.0.1:11434`. Since Obot runs in a container, `localhost` addresses resolve to Obot's container, not your host. Set `OLLAMA_HOST=0.0.0.0` before starting Ollama, then use your host's IP address in the endpoint URL.

2. **Use the OpenAI-compatible endpoint** - The endpoint must include the `/v1/` path:
   ```
   http://<your-host-ip>:11434/v1/
   ```
   Using `http://<host>:11434/` without `/v1/` will result in validation errors.

See [Ollama's FAQ](https://docs.ollama.com/faq) for platform-specific instructions on setting `OLLAMA_HOST`.
