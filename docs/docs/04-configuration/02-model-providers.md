# Model Providers

Each package model provider is configured slightly differently, but, at least, the `OTTO_*_MODEL_PROVIDER_API_KEY` environment variable are required, where `*` stands in for the name of the provider (e.g. `OTTO_AZURE_OPENAI_MODEL_PROVIDER_API_KEY`).

Below is a summary of the configuration options for each provider. However, the packaged model providers are configured to indicate which environment variables are required and which ones are not set in the current environment. For example, `/api/tool-references/azure-openai-model-provider` would indicate the status of the Azure OpenAI model provider. If the environment variables are not set, then the API would return something like:

```json
{
  "id": "azure-openai-model-provider",
  "created": "2024-11-08T16:03:21-05:00",
  "metadata": {
    "envVars": "OTTO_AZURE_OPENAI_MODEL_PROVIDER_API_KEY,OTTO_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT,OTTO_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
  },
  "name": "Azure OpenAI Provider",
  "toolType": "modelProvider",
  "reference": "github.com/otto8-ai/tools/./azure-openai-model-provider",
  "active": true,
  "builtin": true,
  "description": "Model provider for Azure OpenAI hosted models",
  "modelProviderStatus": {
    "missingEnvVars": [
      "OTTO_AZURE_OPENAI_MODEL_PROVIDER_API_KEY",
      "OTTO_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT",
      "OTTO_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
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
    "envVars": "OTTO_AZURE_OPENAI_MODEL_PROVIDER_API_KEY,OTTO_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT,OTTO_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME"
  },
  "name": "Azure OpenAI Provider",
  "toolType": "modelProvider",
  "reference": "github.com/otto8-ai/tools/./azure-openai-model-provider",
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

The OpenAI model provider is the default and is configured by either setting `OPENAI_API_KEY` or `OTTO_OPENAI_MODEL_PROVIDER_API_KEY` environment variables.

## Azure OpenAI

The Azure OpenAI model provider is configured by setting the following environment variables:
- `OTTO_AZURE_OPENAI_MODEL_PROVIDER_API_KEY`: Found on the "Home" page of the Azure OpenAI Studio.
- `OTTO_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENT_NAME`: The name of the deployment to use, found on the "Deployments" page of the Azure OpenAI Studio.
- `OTTO_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT`:  The endpoint to use, found by clicking on the "Deployment" name from the "Deployments" page of the Azure OpenAI Studio.

## Anthropic

The Anthropic model provider is configured by setting the `OTTO_ANTHROPIC_MODEL_PROVIDER_API_KEY` environment variable. An API key for your Anthropic account can be obtained [here](https://console.anthropic.com/settings/keys).
