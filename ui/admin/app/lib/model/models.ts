import { z } from "zod";

import { EntityMeta } from "~/lib/model/primitives";

export type ModelManifest = {
    name?: string;
    targetModel?: string;
    modelProvider: string;
    active: boolean;
    default: boolean;
    usage?: string;
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
    default: z.boolean(),
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
};

export function getModelsForProvider(providerId: string) {
    if (!providerId) return [];

    if (!(providerId in ModelToProviderMap))
        throw new Error(`Unknown provider: ${providerId}`);

    return ModelToProviderMap[providerId as keyof typeof ModelToProviderMap];
}
