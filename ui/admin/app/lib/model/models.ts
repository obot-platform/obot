import { z } from "zod";

import { ModelProviderStatus } from "~/lib/model/modelProviders";
import { EntityMeta } from "~/lib/model/primitives";

export const ModelUsage = {
	LLM: "llm",
	TextEmbedding: "text-embedding",
	ImageGeneration: "image-generation",
	Vision: "vision",
	Other: "other",
	Unknown: "",
} as const;
export type ModelUsage = (typeof ModelUsage)[keyof typeof ModelUsage];

const ModelUsageLabels = {
	[ModelUsage.LLM]: "Language Model (Chat)",
	[ModelUsage.TextEmbedding]: "Text Embedding (Knowledge)",
	[ModelUsage.ImageGeneration]: "Image Generation",
	[ModelUsage.Vision]: "Vision",
	[ModelUsage.Other]: "Other",
	[ModelUsage.Unknown]: "Unknown",
} as const;

export const getModelUsageLabel = (usage: string) => {
	if (!(usage in ModelUsageLabels)) return usage;

	return ModelUsageLabels[usage as ModelUsage];
};

export const ModelAlias = {
	Llm: "llm",
	LlmMini: "llm-mini",
	TextEmbedding: "text-embedding",
	ImageGeneration: "image-generation",
	Vision: "vision",
} as const;
export type ModelAlias = (typeof ModelAlias)[keyof typeof ModelAlias];

const ModelAliasLabels = {
	[ModelAlias.Llm]: "Language Model (Chat)",
	[ModelAlias.LlmMini]: "Language Model (Chat - Fast)",
	[ModelAlias.TextEmbedding]: "Text Embedding (Knowledge)",
	[ModelAlias.ImageGeneration]: "Image Generation",
	[ModelAlias.Vision]: "Vision",
} as const;

export const getModelAliasLabel = (alias: string) => {
	if (!(alias in ModelAliasLabels)) return alias;

	return ModelAliasLabels[alias as ModelAlias];
};

export const ModelAliasToUsageMap = {
	[ModelAlias.Llm]: ModelUsage.LLM,
	[ModelAlias.LlmMini]: ModelUsage.LLM,
	[ModelAlias.TextEmbedding]: ModelUsage.TextEmbedding,
	[ModelAlias.ImageGeneration]: ModelUsage.ImageGeneration,
	[ModelAlias.Vision]: ModelUsage.Vision,
} as const;

export function filterModelsByUsage(
	models: Model[],
	usages: ModelUsage | ModelUsage[],
	sort = (a: Model, b: Model) => (b.name ?? "").localeCompare(a.name ?? "")
) {
	const _usages = Array.isArray(usages) ? usages : [usages];

	// Vision models are LLMs
	if (_usages.includes(ModelUsage.Vision)) {
		_usages.push(ModelUsage.LLM);
	}

	return models.filter((model) => _usages.includes(model.usage)).sort(sort);
}

export function filterModelsByActive(models: Model[]) {
	return models.filter((model) => model.active);
}

export type ModelManifest = {
	name?: string;
	targetModel?: string;
	modelProvider: string;
	active: boolean;
	usage: ModelUsage;
};

export type Model = EntityMeta & ModelManifest;

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

export function getModelUsageFromAlias(alias: string) {
	if (!(alias in ModelAliasToUsageMap)) return null;

	return ModelAliasToUsageMap[alias as keyof typeof ModelAliasToUsageMap];
}
