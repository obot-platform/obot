export const CommonModelProviderIds = {
    OLLAMA: "ollama-model-provider",
    VOYAGE: "voyage-model-provider",
    ANTHROPIC: "anthropic-model-provider",
    OPENAI: "openai-model-provider",
    AZURE_OPENAI: "azure-openai-model-provider",
};

export const ModelProviderLinks = {
    [CommonModelProviderIds.VOYAGE]: "https://www.voyageai.com/",
    [CommonModelProviderIds.OLLAMA]: "https://ollama.com/",
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://azure.microsoft.com/en-us/explore/",
    [CommonModelProviderIds.ANTHROPIC]: "https://www.anthropic.com",
    [CommonModelProviderIds.OPENAI]: "https://openai.com/",
};

export const ModelProviderConfigurationLinks = {
    [CommonModelProviderIds.AZURE_OPENAI]:
        "https://docs.otto8.ai/configuration/model-providers#azure-openai",
};
