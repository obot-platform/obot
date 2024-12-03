import { z } from "zod";

import { EntityMeta } from "~/lib/model/primitives";

export const ModelUsage = {
    LLM: "llm",
    TextEmbedding: "text-embedding",
    ImageGeneration: "image-generation",
    Other: "other",
} as const;
export type ModelUsage = (typeof ModelUsage)[keyof typeof ModelUsage];

const ModelUsageLabels = {
    [ModelUsage.LLM]: "LLM",
    [ModelUsage.TextEmbedding]: "Text Embedding",
    [ModelUsage.ImageGeneration]: "Image Generation",
    [ModelUsage.Other]: "Other",
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
    requiredConfigurationParameters?: string[];
    missingConfigurationParameters?: string[];
};

export type Model = EntityMeta & ModelManifest & ModelProviderStatus;

export const ModelManifestSchema = z.object({
    name: z.string(),
    targetModel: z.string().min(1, "Required"),
    modelProvider: z.string().min(1, "Required"),
    active: z.boolean(),
    usage: z.nativeEnum(ModelUsage),
});

type ModelProviderManifest = {
    name: string;
    toolReference: string;
};

export type ModelProvider = EntityMeta &
    ModelProviderManifest &
    ModelProviderStatus;

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
