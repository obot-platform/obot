import {
	CommonModelProviderIds,
	FEATURED_AGENT_PREFERRED_ORDER,
	MCP_LIST_ORDER
} from './constants';
import type { ModelProvider, ProjectShare, ProjectTemplate } from './services';
import type { MCPCatalogEntry } from './services/admin/types';

export const sortByPreferredMcpOrder = (a: MCPCatalogEntry, b: MCPCatalogEntry) => {
	const aId = (a.id?.toLowerCase() ?? '').trim();
	const bId = (b.id?.toLowerCase() ?? '').trim();
	const aIndex = MCP_LIST_ORDER.indexOf(aId);
	const bIndex = MCP_LIST_ORDER.indexOf(bId);

	if (aIndex === -1 && bIndex === -1) return 0;
	if (aIndex === -1) return 1;
	if (bIndex === -1) return -1;
	return aIndex - bIndex;
};

export const sortByFeaturedNameOrder = (a: ProjectShare, b: ProjectShare) => {
	const aName = (a.name?.toLowerCase() ?? '').trim();
	const bName = (b.name?.toLowerCase() ?? '').trim();
	const aIndex = FEATURED_AGENT_PREFERRED_ORDER.indexOf(aName);
	const bIndex = FEATURED_AGENT_PREFERRED_ORDER.indexOf(bName);
	if (aIndex === -1 && bIndex === -1) return 0;
	if (aIndex === -1) return 1;
	if (bIndex === -1) return -1;
	return aIndex - bIndex;
};

export const sortTemplatesByFeaturedNameOrder = (a: ProjectTemplate, b: ProjectTemplate) => {
	const aName = (a.name?.toLowerCase() ?? '').trim();
	const bName = (b.name?.toLowerCase() ?? '').trim();
	const aIndex = FEATURED_AGENT_PREFERRED_ORDER.indexOf(aName);
	const bIndex = FEATURED_AGENT_PREFERRED_ORDER.indexOf(bName);
	if (aIndex === -1 && bIndex === -1) return 0;
	if (aIndex === -1) return 1;
	if (bIndex === -1) return -1;
	return aIndex - bIndex;
};

export const sortShownToolsPriority = (a: string, b: string) => {
	const lastPriorityTools = new Set([
		'images-analyze-images',
		'images-generate-images',
		'obot-search'
	]);
	const aIsLastPriority = lastPriorityTools.has(a);
	const bIsLastPriority = lastPriorityTools.has(b);
	if (aIsLastPriority && !bIsLastPriority) return 1;
	if (!aIsLastPriority && bIsLastPriority) return -1;
	return 0;
};

export const sortByCreatedDate = <T extends { created: string }>(a: T, b: T) => {
	return new Date(b.created).getTime() - new Date(a.created).getTime();
};

export const sortModelProviders = (modelProviders: ModelProvider[]) => {
	return [...modelProviders].sort((a, b) => {
		const preferredOrder = [
			CommonModelProviderIds.OPENAI,
			CommonModelProviderIds.AZURE_OPENAI,
			CommonModelProviderIds.ANTHROPIC,
			CommonModelProviderIds.ANTHROPIC_BEDROCK,
			CommonModelProviderIds.XAI,
			CommonModelProviderIds.OLLAMA,
			CommonModelProviderIds.VOYAGE,
			CommonModelProviderIds.GROQ,
			CommonModelProviderIds.VLLM,
			CommonModelProviderIds.DEEPSEEK,
			CommonModelProviderIds.GEMINI_VERTEX,
			CommonModelProviderIds.GENERIC_OPENAI
		];
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
