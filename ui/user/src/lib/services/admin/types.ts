export interface MCPCatalogManifest {
	displayName: string;
	sourceURLs: string[];
	allowedUserIDs: string[];
}

export interface MCPCatalog extends MCPCatalogManifest {
	id: string;
}

export interface MCPCatalogSource {
	id: string;
}

export interface MCPCatalogEntryServerManifest {
	icon?: string;
	args?: string[];
	env?: MCPCatalogEntryFieldManifest[];
	command?: string;
	fixedURL?: string;
	hostname?: string;
	headers?: MCPCatalogEntryFieldManifest[];
	name?: string;
	description?: string;
	metadata?: {
		categories?: string;
	};
}

export interface MCPCatalogEntry {
	id: string;
	created: string;
	commandManifest?: MCPCatalogEntryServerManifest;
	urlManifest?: MCPCatalogEntryServerManifest;
	sourceURL?: string;
	type: string;
}

export interface MCPCatalogEntryFieldManifest {
	key: string;
	description: string;
	name: string;
	required: boolean;
	sensitive: boolean;
	value: string;
}

export type MCPCatalogEntryFormData = Omit<MCPCatalogEntryServerManifest, 'metadata'> & {
	categories: string[];
	url?: string;
};

export interface MCPCatalogServerManifest {
	catalogEntryID?: string;
	manifest: Omit<MCPCatalogEntryServerManifest, 'fixedURL'> & {
		url?: string;
	};
}

export interface OrgUser {
	created: string;
	username: string;
	email: string;
	explicitAdmin: boolean;
	role: number;
	iconURL: string;
	id: string;
	lastActiveDay?: string;
}

export const Role = {
	ADMIN: 1,
	USER: 10
};

export interface ProviderParameter {
	name: string;
	friendlyName?: string;
	description?: string;
	sensitive?: boolean;
	hidden?: boolean;
}

export interface BaseProvider {
	name: string;
	configured: boolean;
	created: string;
	missingConfigurationParameters?: string[];
	optionalConfigurationParameters?: ProviderParameter[];
	requiredConfigurationParameters?: ProviderParameter[];
	icon?: string;
	iconDark?: string;
	id: string;
	link?: string;
	namespace?: string;
	toolReference?: string;
}

export interface AuthProvider extends BaseProvider {
	type: 'authprovider';
}

export interface FileScannerProvider extends BaseProvider {
	type: 'filescannerprovider';
}

export interface FileScannerConfig {
	id: string;
	providerName: string;
	providerNamespace: string;
	updatedAt: string;
}

export interface Model {
	active: boolean;
	aliasAssigned: boolean;
	created: string;
	id: string;
	modelProvider: string;
	modelProviderName: string;
	name: string;
	targetModel: string;
	usage: string;
}

interface BaseThread {
	created: string;
	id: string;
	name: string;
	currentRunId?: string;
	projectID?: string;
	lastRunID?: string;
	userID?: string;
	project?: boolean;
	deleted?: string;
	systemTask?: boolean;
	ready?: boolean;
}

export type ProjectThread = BaseThread &
	({ assistantID: string; taskID?: never } | { assistantID?: never; taskID: string });

export const ModelUsage = {
	LLM: 'llm',
	TextEmbedding: 'text-embedding',
	ImageGeneration: 'image-generation',
	Vision: 'vision',
	Other: 'other',
	Unknown: ''
} as const;
export type ModelUsage = (typeof ModelUsage)[keyof typeof ModelUsage];

export const ModelUsageLabels = {
	[ModelUsage.LLM]: 'Language Model (Chat)',
	[ModelUsage.TextEmbedding]: 'Text Embedding (Knowledge)',
	[ModelUsage.ImageGeneration]: 'Image Generation',
	[ModelUsage.Vision]: 'Vision',
	[ModelUsage.Other]: 'Other',
	[ModelUsage.Unknown]: 'Unknown'
} as const;

export const ModelAlias = {
	Llm: 'llm',
	LlmMini: 'llm-mini',
	TextEmbedding: 'text-embedding',
	ImageGeneration: 'image-generation',
	Vision: 'vision'
} as const;
export type ModelAlias = (typeof ModelAlias)[keyof typeof ModelAlias];

export const ModelAliasLabels = {
	[ModelAlias.Llm]: 'Language Model (Chat)',
	[ModelAlias.LlmMini]: 'Language Model (Chat - Fast)',
	[ModelAlias.TextEmbedding]: 'Text Embedding (Knowledge)',
	[ModelAlias.ImageGeneration]: 'Image Generation',
	[ModelAlias.Vision]: 'Vision'
} as const;

export const ModelAliasToUsageMap = {
	[ModelAlias.Llm]: ModelUsage.LLM,
	[ModelAlias.LlmMini]: ModelUsage.LLM,
	[ModelAlias.TextEmbedding]: ModelUsage.TextEmbedding,
	[ModelAlias.ImageGeneration]: ModelUsage.ImageGeneration,
	[ModelAlias.Vision]: ModelUsage.Vision
} as const;

export interface DefaultModelAlias {
	alias: ModelAlias;
	model: string;
}

export interface AccessControlRule {
	id: string;
}

export interface AccessControlRuleManifest {
	userIDs: string[];
}
