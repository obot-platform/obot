import { z } from "zod";

import { EntityMeta } from "~/lib/model/primitives";

export const ModelUsage = {
    LLM: "llm",
    TextEmbedding: "text-embedding",
    ImageGeneration: "image-generation",
} as const;
export type ModelUsage = (typeof ModelUsage)[keyof typeof ModelUsage];

const ModelUsageLabels = {
    [ModelUsage.LLM]: "LLM",
    [ModelUsage.TextEmbedding]: "Text Embedding",
    [ModelUsage.ImageGeneration]: "Image Generation",
} as const;

export const getModelUsageLabel = (usage: string) => {
    if (!(usage in ModelUsageLabels)) return usage;

    return ModelUsageLabels[usage as ModelUsage];
};

export type ModelManifest = {
    name?: string;
    targetModel?: string;
    modelProvider: string;
    active: boolean;
    usage: ModelUsage;
};

export type ModelProviderStatus = {
    configured: boolean;
    missingEnvVars?: string[];
};

export type Model = EntityMeta & ModelManifest & ModelProviderStatus;

export const ModelManifestSchema = z.object({
    name: z.string(),
    targetModel: z.string().min(1, "Required"),
    modelProvider: z.string().min(1, "Required"),
    active: z.boolean(),
    usage: z.nativeEnum(ModelUsage),
});

export type ModelProvider = EntityMeta & {
    description?: string;
    builtin: boolean;
    active: boolean;
    modelProviderStatus: ModelProviderStatus;
    name: string;
    reference: string;
    toolType: "modelProvider";
};

// note(ryanhopperlowe): these values are hardcoded for now
// ideally they should come from the backend
const ModelToProviderMap = {
    "openai-model-provider": [
        "text-embedding-3-small",
        "dall-e-3",
        "gpt-4o-mini",
        "gpt-3.5-turbo",
        "text-embedding-ada-002",
        "gpt-4o",
    ],
    "azure-openai-model-provider": [
        "text-embedding-3-small",
        "dall-e-3",
        "gpt-4o-mini",
        "gpt-3.5-turbo",
        "text-embedding-ada-002",
        "gpt-4o",
    ],
    "anthropic-model-provider": [
        "claude-3-opus-latest",
        "claude-3-5-sonnet-latest",
        "claude-3-5-haiku-latest",
    ],
    "ollama-model-provider": ["llama3.2"],
    "voyage-model-provider": [
        "voyage-3",
        "voyage-3-lite",
        "voyage-finance-2",
        "voyage-multilingual-2",
        "voyage-law-2",
        "voyage-code-2",
    ],
};

export const ModelAliasToUsageMap = {
    llm: ModelUsage.LLM,
    "llm-mini": ModelUsage.LLM,
    "text-embedding": ModelUsage.TextEmbedding,
    "image-generation": ModelUsage.ImageGeneration,
} as const;

export function getModelUsageFromAlias(alias: string) {
    if (!(alias in ModelAliasToUsageMap)) return null;

    return ModelAliasToUsageMap[alias as keyof typeof ModelAliasToUsageMap];
}

export function getModelsForProvider(providerId: string) {
    if (!providerId || !(providerId in ModelToProviderMap)) return [];
    return ModelToProviderMap[providerId as keyof typeof ModelToProviderMap];
}
