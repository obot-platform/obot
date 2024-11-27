# Model Providers

Each packaged model provider has a slightly different configuration set via environment variables. The environment variables for the packaged model providers are of the form `OTTO8_*_MODEL_PROVIDER_CONFIG_ITEM` where `*` stands in for the name of the provider (e.g. `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_API_KEY`).

Below is a summary of the configuration options for each provider. However, the packaged model providers indicate which environment variables they require and which ones aren't set in the current environment. For example, `/api/tool-references/azure-openai-model-provider` would indicate the status of the Azure OpenAI model provider. If you don't set the environment variables, then the API would return something like:

```json
{
  "id": "azure-openai-model-provider",
  "created": "2024-11-08T16:03:21-05:00",
  "metadata": {
    "envVars": "OTTO8_AZURE_OPENAI_MODEL_PROVIDER_API_KEY,OTTO8_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT,OTTO8_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
  },
  "name": "Azure OpenAI Provider",
  "toolType": "modelProvider",
  "reference": "github.com/otto8-ai/tools/azure-openai-model-provider",
  "active": true,
  "builtin": true,
  "description": "Model provider for Azure OpenAI hosted models",
  "modelProviderStatus": {
    "missingEnvVars": [
      "OTTO8_AZURE_OPENAI_MODEL_PROVIDER_API_KEY",
      "OTTO8_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT",
      "OTTO8_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
    ],  
    "configured": false
  }
}

```

Once all the required environment variables are set, then the API would return something like:

```json
{
  "id": "azure-openai-model-provider",
  "created": "2024-11-08T16:03:21-05:00",
  "metadata": {
    "envVars": "OTTO8_AZURE_OPENAI_MODEL_PROVIDER_API_KEY,OTTO8_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT,OTTO8_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
  },
  "name": "Azure OpenAI Provider",
  "toolType": "modelProvider",
  "reference": "github.com/otto8-ai/tools/azure-openai-model-provider",
  "active": true,
  "builtin": true,
  "description": "Model provider for Azure OpenAI hosted models",
  "modelProviderStatus": {
    "configured": true
  }
}
```

The UI also uses this API to indicate the status of the model provider.

## OpenAI

The OpenAI model provider is the default and is configured by either setting `OPENAI_API_KEY` or `OTTO8_OPENAI_MODEL_PROVIDER_API_KEY` environment variables.

## Azure OpenAI

The Azure OpenAI model provider requires setting the following environment variables:
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_API_KEY`: Found on the "Home" page of the Azure OpenAI Studio.
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT`:  The endpoint to use, found by clicking on the "Deployment" name from the "Deployments" page of the Azure OpenAI Studio.
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_ID`: The resource group name for the Azure OpenAI resource, found by click on the resource name in the top-right of the Azure OpenAI Studio.

The remainder of these environment variables are used to list the available deployments in Azure. A service principal must be created with read permission to the `Microsoft.CognitiveServices`. At the time of this writing, a built-in role doesn't exist in Azure. One can be created with the following permissions:
```json
"permissions": [
	{
		"actions": [
			"Microsoft.CognitiveServices/*/read"
		], 
		"notActions": [],
		"dataActions": [],
		"notDataActions": []
	}
]
```

After this service principal is created, the following environment variables are required to configure the model provider in Otto8:
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID`: The client ID for the app registration.
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET`: The client secret for the app registration.
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID`: The tenant ID for the app registration.
- `OTTO8_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID`: The subscription ID for the Azure account.

:::note
When configuring models with the Azure OpenAI provider in Otto8, the "Target Model" should be the "Deployment" from Azure.
:::

## Anthropic

The Anthropic model provider requires setting the `OTTO8_ANTHROPIC_MODEL_PROVIDER_API_KEY` environment variable. You can get an API key for your Anthropic account [here](https://console.anthropic.com/settings/keys).

## Voyage AI

Voyage is Anthropic's recommended text-embedding provider. The Voyage model provider requires setting the `OTTO8_VOYAGE_MODEL_PROVIDER_API_KEY` environment variable. You can get an API key for your Voyage account [here](https://dash.voyageai.com/api-keys).

## Ollama

The Ollama model provider requires the `OTTO8_OLLAMA_MODEL_PROVIDER_HOST` environment variable. This host must point to a running instance of Ollama. For your reference, the default host and port for Ollama is `127.0.0.1:11434`. Otto8 doesn't set this by default.

To set up and run an instance of Ollama, refer to the [Ollama GitHub readme](https://github.com/ollama/ollama/blob/main/README.md).
