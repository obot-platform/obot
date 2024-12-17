export const CommonModelProviderIds = {
    OLLAMA: "ollama-model-provider",
    GROQ: "groq-model-provider",
    VOYAGE: "voyage-model-provider",
    ANTHROPIC: "anthropic-model-provider",
    OPENAI: "openai-model-provider",
    AZURE_OPENAI: "azure-openai-model-provider",
};

export const ModelProviderLinks = {
    [CommonModelProviderIds.VOYAGE]: "https://www.voyageai.com/",
    [CommonModelProviderIds.OLLAMA]: "https://ollama.com/",
    [CommonModelProviderIds.GROQ]: "https://groq.com/",
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://azure.microsoft.com/en-us/explore/",
    [CommonModelProviderIds.ANTHROPIC]: "https://www.anthropic.com",
    [CommonModelProviderIds.OPENAI]: "https://openai.com/",
};

export const ModelProviderConfigurationLinks = {
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://docs.otto8.ai/configuration/model-providers#azure-openai",
};

export const RecommendedModelProviders = [
    CommonModelProviderIds.OPENAI,
    CommonModelProviderIds.AZURE_OPENAI,
];

export const ModelProviderRequiredTooltips: {
    [key: string]: {
        [key: string]: string;
    };
} = {
    [CommonModelProviderIds.OLLAMA]: {
        Host: "IP Address for the ollama server (eg. 127.0.0.1:1234)",
    },
    [CommonModelProviderIds.GROQ]: {
        "API Key":
            "Groq API Key. Can be created and fetched from https://console.groq.com/keys",
    },
    [CommonModelProviderIds.AZURE_OPENAI]: {
        Endpoint:
            "Endpoint for the Azure OpenAI service (eg. https://<resource-name>.<region>.api.cognitive.microsoft.com/)",
        "Client Id":
            "Unique identifier for the application when using Azure Active Directory. Can typically be found in App Registrations > [application].",
        "Client Secret":
            "Password or key that app uses to authenticate with Azure Active Directory. Can typically be found in App Registrations > [application] > Certificates & Secrets",
        "Tenant Id":
            "Identifier of instance where the app and resources reside. Can typically be found in Azure Active Directory > Overview > Directory ID",
        "Subscription Id":
            "Identifier of user's Azure subscription. Can typically be found in Azure Portal > Subscriptions > Overview.",
        "Resource Group":
            "Container that holds related Azure resources. Can typically be found in Azure Portal > Resource Groups > [OpenAI Resource Group] > Overview",
    },
};

export const ModelProviderSensitiveFields: Record<string, boolean | undefined> =
    {
        // OpenAI
        OBOT_OPENAI_MODEL_PROVIDER_API_KEY: true,

        // Azure OpenAI
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET: true,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID: false,
        OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP: false,

        // Anthropic
        OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY: true,

        // Voyage
        OBOT_VOYAGE_MODEL_PROVIDER_API_KEY: true,

        // Ollama
        OBOT_OLLAMA_MODEL_PROVIDER_HOST: true,

        // Groq
        OBOT_GROQ_MODEL_PROVIDER_API_KEY: true,
    };
