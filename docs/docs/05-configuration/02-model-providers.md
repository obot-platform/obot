# Model Providers

Each packaged model provider has a slightly different configuration.
Below is a summary of the configuration options for each provider.
However, the packaged model providers indicate which configuration parameters they require, and which ones aren't set in the current environment.
For example, `/api/model-providers/azure-openai-model-provider` would indicate the status of the Azure OpenAI model provider.
If this model provider hasn't been configured, then the API would return something like:

```json
{
	"id": "azure-openai-model-provider",
	"created": "2024-12-02T08:33:34-05:00",
	"revision": "1033",
	"type": "modelprovider",
	"name": "azure-openai-model-provider",
	"toolReference": "github.com/obot-platform/tools/azure-openai-model-provider",
	"configured": false,
	"requiredConfigurationParameters": [
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP"
	],
	"missingConfigurationParameters": [
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP"
	]
}

```

To configure a model provider using the API, a `POST` request can be made to `/api/model-providers/azure-openai-model-provider/configure` with each required configuration parameter set in the body:

```json
{
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT": "...",
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID": "...",
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET": "...",
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID": "...",
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID": "...",
	"OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP": "..."
}
```

Once the model provider has been configured, then the API would return something like:

```json
{
	"id": "azure-openai-model-provider",
	"created": "2024-12-02T08:33:34-05:00",
	"revision": "1033",
	"type": "modelprovider",
	"name": "azure-openai-model-provider",
	"toolReference": "github.com/obot-platform/tools/azure-openai-model-provider",
	"configured": true,
	"requiredConfigurationParameters": [
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID",
		"OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP"
	]
}
```

The UI also uses this API to indicate the status of the model provider.

## OpenAI

The OpenAI model provider is the default and has one required configuration parameter: `OBOT_OPENAI_MODEL_PROVIDER_API_KEY`.

The OpenAI model provider is also special: you can start Obot with the `OPENAI_API_KEY` environment variable set and Obot will automatically configure the OpenAI model provider.

## Azure OpenAI

The Azure OpenAI model provider requires the following configuration parameters:
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT`:  The endpoint to use, found by clicking on the "Deployment" name from the "Deployments" page of the Azure OpenAI Studio. The provider endpoint must be in the format `https://<your-custom-name>.openai.azure.com` - if your Azure OpenAI resource doesn't have an endpoint that looks like this, you need to create one.
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP`: The resource group name for the Azure OpenAI resource, found by clicking on the resource name in the top-right of the Azure OpenAI Studio.

A service principal must be created with the (equivalent permissions of the) `Cognitive Services OpenAI User`.  After this service principal is created, use the following parameters to configure the model provider in Obot:
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID`: The client ID for the app registration.
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET`: The client secret for the app registration.
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID`: The tenant ID for the app registration.
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID`: The subscription ID for the Azure account.
- `OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_VERSION`: (optional) Specify the API version to use with Azure OpenAI instead of `2024-10-21`.

:::note
When configuring models with the Azure OpenAI provider in Obot, the "Target Model" should be the "Deployment" from Azure.
:::

## Anthropic

The Anthropic model provider requires one configuration parameter: `OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY`. You can get an API key for your Anthropic account [here](https://console.anthropic.com/settings/keys).

## Voyage AI

Voyage is Anthropic's recommended text-embedding provider. The Voyage model provider requires `OBOT_VOYAGE_MODEL_PROVIDER_API_KEY`. You can get an API key for your Voyage account [here](https://dash.voyageai.com/api-keys).

## Ollama

The Ollama model provider requires the configuration parameter `OBOT_OLLAMA_MODEL_PROVIDER_HOST`. This host must point to a running instance of Ollama. For your reference, the default host and port for Ollama is `127.0.0.1:11434`. Obot doesn't set this by default.

To set up and run an instance of Ollama, refer to the [Ollama GitHub readme](https://github.com/ollama/ollama/blob/main/README.md).

## vLLM

The vLLM model provider requires the following configuration parameters:

- `OBOT_VLLM_MODEL_PROVIDER_ENDPOINT`: The endpoint to use for your vLLM server (e.g., `your-vllm-server.com` or `https://your-vllm-server.com`)
- `OBOT_VLLM_MODEL_PROVIDER_API_KEY`: The API key for authentication with your vLLM server
