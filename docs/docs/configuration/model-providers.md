# Model Providers

The Model Providers page allows administrators to configure and manage various AI model providers. This guide will walk you through the setup process and explain the available options.

### Configuring Model Providers

Obot supports a variety of model providers, including:

- OpenAI
- Anthropic
- xAI
- Ollama
- Voyage AI
- Groq
- vLLM
- DeepSeek
- Google

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

#### Azure OpenAI (Enterprise only)

The Azure OpenAI model provider supports legacy Azure OpenAI resources. Microsoft Foundry works with API key authentication as well.

This model provider supports two forms of authentication: API keys and Microsoft Entra.

##### API Key Authentication

In the Azure OpenAI or Microsoft Foundry UI, you can find your API key after you have set up at least one deployment, as well as your endpoint URL.
Both of these values are required when you configure this model provider in Obot.

Additionally, you must manually specify the names of all of your deployments, as the API key does not provide the ability to list them.
The format is `name:type`, for example, `gpt-5.2:reasoning-llm`. The supported types are `llm`, `reasoning-llm`, `text-embedding`, and `image-generation`.
If no type is specified, Obot will assume the type is `llm`.

If the type specified is `llm` or none at all, and the deployment name starts with the name of a known reasoning model (such as `o3` or `gpt-5`), Obot
will automatically treat it as a reasoning model.

##### Microsoft Entra Authentication

:::important
At this time, Microsoft Entra authentication is only supported for Azure OpenAI deployments and not for the newer Microsoft Foundry deployments.
:::

Instead of using an API key, you can set up a Microsoft Entra app registration as a service principal to use Azure OpenAI.

Obot requires the Client ID, Client Secret, and Tenant ID of the Entra app, as well as the Endpoint URL, Subscription ID, and Resource Group from Azure OpenAI/Microsoft Foundry.

You do NOT need to manually specify your deployment names, as the Entra app's credentials will be sufficient to list them.

After you have created your Entra app registration, you need to go to your Azure OpenAI resource in the Azure portal and add a new role assignment for the app registration, as a service principal.
It needs to have the `Cognitive Services OpenAI User` role.

See the [Microsoft docs](https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/role-based-access-control?view=foundry-classic#add-role-assignment-to-an-azure-openai-resource) for more details.
