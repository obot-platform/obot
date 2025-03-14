# Model Providers

The Model Providers page allows administrators to configure and manage various AI model providers. This guide will walk you through the setup process and explain the available options.

## Configuring Model Providers

Obot supports a variety of model providers, including:

- OpenAI
- Anthropic
- xAI
- Ollama
- Voyage AI
- Groq
- vLLM
- DeepSeek

The UI will indicate whether each provider has been configured. If a provider is configured, options to view and manage its models and deconfigure it will be available.

:::note
Our Enterprise release adds support for additional Enterprise-grade model providers. [See here](/enterprise) for more details.
:::

### Configuring and enabling a provider

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

These defaults ensure that users have pre-selected models for the tools and other functionality throughout the platform. After selecting the desired defaults, click "Save Changes" to confirm your configurations.
