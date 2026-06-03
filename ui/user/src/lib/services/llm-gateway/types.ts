import { CommonModelProviderIds } from '$lib/constants';

export type ProviderShortKey = 'openai' | 'anthropic';

export interface ProviderConnection {
	id: string;
	shortKey: ProviderShortKey;
	displayName: string;
}

export const PROVIDER_CONNECTIONS: Record<ProviderShortKey, ProviderConnection> = {
	openai: {
		id: CommonModelProviderIds.OPENAI,
		shortKey: 'openai',
		displayName: 'OpenAI'
	},
	anthropic: {
		id: CommonModelProviderIds.ANTHROPIC,
		shortKey: 'anthropic',
		displayName: 'Anthropic'
	}
};

export const SUPPORTED_PROVIDER_IDS = new Set<string>([
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.ANTHROPIC
]);

export interface RenderContext {
	provider: ProviderConnection;
	/** e.g. https://obot.example.com */
	obotURL: string;
	/** e.g. https://obot.example.com/api/llm-proxy/anthropic */
	baseURL: string;
	/** First available model name for the provider, used in example invocations. */
	exampleModel?: string;
}

export interface SnippetBlock {
	/** Optional label rendered above the code block. */
	title?: string;
	language: 'bash' | 'json' | 'toml';
	code: string;
}
