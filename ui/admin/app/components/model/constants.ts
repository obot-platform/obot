import { ModelAlias } from "~/lib/model/models";

export const SUGGESTED_MODEL_SELECTIONS: Record<
	ModelAlias,
	string | undefined
> = {
	[ModelAlias.Llm]: "gpt-4.1",
	[ModelAlias.LlmMini]: "gpt-4.1-mini",
	[ModelAlias.TextEmbedding]: "text-embedding-3-large",
	[ModelAlias.ImageGeneration]: "dall-e-3",
	[ModelAlias.Vision]: "gpt-4.1",
};
