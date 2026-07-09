import { CommonModelProviderIds } from '$lib/constants';

export type ProviderShortKey =
	| 'openai'
	| 'anthropic'
	| 'aws-bedrock-anthropic'
	| 'aws-bedrock-openai'
	| 'aws-bedrock-api-key-anthropic'
	| 'aws-bedrock-api-key-openai';

export interface ProviderConnection {
	id: string;
	shortKey: ProviderShortKey;
	displayName: string;
	routePath: string;
}

export const PROVIDER_CONNECTIONS: Record<ProviderShortKey, ProviderConnection> = {
	openai: {
		id: CommonModelProviderIds.OPENAI,
		shortKey: 'openai',
		displayName: 'OpenAI',
		routePath: 'openai'
	},
	anthropic: {
		id: CommonModelProviderIds.ANTHROPIC,
		shortKey: 'anthropic',
		displayName: 'Anthropic',
		routePath: 'anthropic'
	},
	'aws-bedrock-anthropic': {
		id: CommonModelProviderIds.AMAZON_BEDROCK,
		shortKey: 'aws-bedrock-anthropic',
		displayName: 'Amazon Bedrock (Anthropic-compatible)',
		routePath: 'aws-bedrock/anthropic'
	},
	'aws-bedrock-openai': {
		id: CommonModelProviderIds.AMAZON_BEDROCK,
		shortKey: 'aws-bedrock-openai',
		displayName: 'Amazon Bedrock (OpenAI-compatible)',
		routePath: 'aws-bedrock/openai'
	},
	'aws-bedrock-api-key-anthropic': {
		id: CommonModelProviderIds.AMAZON_BEDROCK_API_KEY,
		shortKey: 'aws-bedrock-api-key-anthropic',
		displayName: 'Amazon Bedrock API Key (Anthropic-compatible)',
		routePath: 'aws-bedrock-api-key/anthropic'
	},
	'aws-bedrock-api-key-openai': {
		id: CommonModelProviderIds.AMAZON_BEDROCK_API_KEY,
		shortKey: 'aws-bedrock-api-key-openai',
		displayName: 'Amazon Bedrock API Key (OpenAI-compatible)',
		routePath: 'aws-bedrock-api-key/openai'
	}
};

export const SUPPORTED_PROVIDER_IDS = new Set<string>([
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.ANTHROPIC,
	CommonModelProviderIds.AMAZON_BEDROCK,
	CommonModelProviderIds.AMAZON_BEDROCK_API_KEY
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
