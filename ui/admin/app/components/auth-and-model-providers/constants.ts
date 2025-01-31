export const CommonModelProviderIds = {
	OLLAMA: "ollama-model-provider",
	GROQ: "groq-model-provider",
	VLLM: "vllm-model-provider",
	VOYAGE: "voyage-model-provider",
	ANTHROPIC: "anthropic-model-provider",
	OPENAI: "openai-model-provider",
	AZURE_OPENAI: "azure-openai-model-provider",
	ANTHROPIC_BEDROCK: "anthropic-bedrock-model-provider",
	XAI: "xai-model-provider",
	DEEPSEEK: "deepseek-model-provider",
	GEMINI_VERTEX: "gemini-vertex-model-provider",
	GENERIC_OPENAI: "generic-openai-model-provider",
};

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.AZURE_OPENAI,
];

export const CommonAuthProviderIds = {
	GOOGLE: "google-auth-provider",
	GITHUB: "github-auth-provider",
	OKTA: "okta-auth-provider",
};
