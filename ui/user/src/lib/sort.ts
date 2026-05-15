import { CommonModelProviderIds } from './constants';
import type { ModelProvider } from './services';

export const sortByCreatedDate = <T extends { created: string }>(a: T, b: T) => {
	return new Date(b.created).getTime() - new Date(a.created).getTime();
};

export const sortModelProviders = (modelProviders: ModelProvider[]) => {
	const preferredOrder = [
		CommonModelProviderIds.OPENAI,
		CommonModelProviderIds.ANTHROPIC,
		CommonModelProviderIds.AZURE_OPENAI,
		CommonModelProviderIds.AZURE,
		CommonModelProviderIds.AZURE_ENTRA,
		CommonModelProviderIds.AMAZON_BEDROCK,
		CommonModelProviderIds.AMAZON_BEDROCK_API_KEY,
		CommonModelProviderIds.ANTHROPIC_BEDROCK,
		CommonModelProviderIds.XAI,
		CommonModelProviderIds.OLLAMA,
		CommonModelProviderIds.GROQ,
		CommonModelProviderIds.VLLM,
		CommonModelProviderIds.DEEPSEEK,
		CommonModelProviderIds.GEMINI_VERTEX,
		CommonModelProviderIds.GENERIC_OPENAI
	];
	return [...modelProviders].sort((a, b) => {
		const aIndex = preferredOrder.indexOf(a.id);
		const bIndex = preferredOrder.indexOf(b.id);

		// If both providers are in preferredOrder, sort by their order
		if (aIndex !== -1 && bIndex !== -1) {
			return aIndex - bIndex;
		}

		// If only a is in preferredOrder, it comes first
		if (aIndex !== -1) return -1;
		// If only b is in preferredOrder, it comes first
		if (bIndex !== -1) return 1;

		return a.id.localeCompare(b.id);
	});
};
