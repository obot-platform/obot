import {
	type MCPServerTool,
	type Project,
	type Runtime,
	type UVXRuntimeConfig,
	type NPXRuntimeConfig,
	type ContainerizedRuntimeConfig,
	type Task
} from '../chat/types';

export interface MCPCatalogManifest {
	displayName: string;
	sourceURLs: string[];
	allowedUserIDs: string[];
}

export interface MCPCatalog extends MCPCatalogManifest {
	id: string;
	syncErrors?: Record<string, string>;
	isSyncing?: boolean;
}

export interface MCPCatalogSource {
	id: string;
}

export interface RemoteRuntimeConfigAdmin {
	url: string;
	headers?: MCPCatalogEntryFieldManifest[];
}

export interface RemoteCatalogConfigAdmin {
	fixedURL?: string;
	hostname?: string;
	headers?: MCPCatalogEntryFieldManifest[];
}

export interface MCPCatalogEntryServerManifest {
	icon?: string;
	env?: MCPCatalogEntryFieldManifest[];
	repoURL?: string;
	name?: string;
	description?: string;
	toolPreview?: MCPServerTool[];
	metadata?: {
		categories?: string;
		'allow-multiple'?: string;
	};

	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteCatalogConfigAdmin;
}

export interface MCPCatalogEntry {
	id: string;
	created: string;
	deleted?: string;
	manifest: MCPCatalogEntryServerManifest;
	sourceURL?: string;
	userCount?: number;
	type: string;
	powerUserID?: string;
	powerUserWorkspaceID?: string;
	isCatalogEntry: boolean;
}

export interface MCPCatalogEntryFieldManifest {
	key: string;
	description: string;
	name: string;
	required: boolean;
	sensitive: boolean;
	value: string;
	file?: boolean;
}

export type MCPCatalogEntryFormData = Omit<MCPCatalogEntryServerManifest, 'metadata'> & {
	categories: string[];
	url?: string;
};

// New runtime-based form data structure
export interface RuntimeFormData {
	// Common fields
	name: string;
	description: string;
	icon: string;
	categories: string[];
	env: MCPCatalogEntryFieldManifest[];

	// Runtime selection
	runtime: Runtime;

	// Runtime-specific configs (only one populated based on runtime)
	npxConfig?: NPXRuntimeConfig;
	uvxConfig?: UVXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteCatalogConfigAdmin; // For catalog entries
	remoteServerConfig?: RemoteRuntimeConfigAdmin; // For servers
}

export interface MCPCatalogServerManifest {
	catalogEntryID?: string;
	manifest: Omit<MCPCatalogEntryServerManifest, 'remoteConfig'> & {
		remoteConfig?: RemoteRuntimeConfigAdmin;
	};
}

export interface OrgUser {
	created: string;
	username: string;
	email: string;
	explicitRole: boolean;
	role: number;
	groups: string[];
	iconURL: string;
	id: string;
	lastActiveDay?: string;
	displayName?: string;
	deletedAt?: string;
	originalEmail?: string;
	originalUsername?: string;
}

export interface OrgGroup {
	id: string;
	name: string;
	iconURL?: string;
}

export const Role = {
	BASIC: 4,
	OWNER: 8,
	ADMIN: 16,
	AUDITOR: 32,
	POWERUSER: 64,
	POWERUSER_PLUS: 128
};

export const Group = {
	OWNER: 'owner',
	ADMIN: 'admin',
	POWERUSER_PLUS: 'power-user-plus',
	POWERUSER: 'power-user',
	USER: 'user',
	AUDITOR: 'auditor'
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
	(
		| { assistantID: string; taskID?: never; taskRunID?: never }
		| { assistantID?: never; taskID: string; taskRunID?: string }
	);

export type ProjectTask = Task & {
	created: string;
};

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

export interface AccessControlRuleResource {
	type: 'mcpServerCatalogEntry' | 'mcpServer' | 'selector';
	id: string;
}

export interface AccessControlRuleSubject {
	type: 'user' | 'group' | 'selector';
	id: string;
}

export interface AccessControlRuleManifest {
	id?: string;
	displayName: string;
	subjects?: AccessControlRuleSubject[];
	resources?: AccessControlRuleResource[];
}

export interface AccessControlRule extends Omit<AccessControlRuleManifest, 'id'> {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	powerUserID?: string;
	powerUserWorkspaceID?: string;
}

export interface BootstrapStatus {
	enabled: boolean;
}

export type AuditLogClient = {
	name: string;
	version: string;
};

export interface AuditLog {
	id: string;
	createdAt: string;
	userID: string;
	userAgent?: string;
	mcpServerInstanceName: string;
	mcpServerName: string;
	mcpServerDisplayName: string;
	mcpServerCatalogEntryName?: string;
	mcpID?: string;
	client: AuditLogClient;
	clientIP: string;
	callType: string;
	callIdentifier?: string;
	responseStatus: number;
	processingTimeMs: number;
	requestHeaders?: Record<string, string | string[]>;
	requestBody?: {
		capabilities?: Record<string, unknown>;
		clientInfo?: Record<string, string>;
		protocolVersion?: string;
	};
	responseHeaders?: Record<string, string | string[]>;
	responseBody?: {
		tools?: Record<string, unknown>[];
		prompts?: Record<string, unknown>[];
		resources?: Record<string, unknown>[];
	};
	error?: string;
	sessionID?: string;
	requestID?: string;
}

export interface AuditLogToolCallStatItem {
	createdAt: string;
	userID: string;
	processingTimeMs: number;
	responseStatus: number;
	error: string;
}

export interface AuditLogToolCallStat {
	toolName: string;
	callCount: number;
	items?: AuditLogToolCallStatItem[];
}

export interface AuditLogResourceReadStat {
	resourceUri: string;
	readCount: number;
}

export interface AuditLogPromptReadStat {
	promptName: string;
	readCount: number;
}

export interface AuthLogUsageStatItem {
	mcpID: string;
	mcpServerInstanceName: string;
	mcpServerName: string;
	mcpServerDisplayName: string;
	toolCalls?: AuditLogToolCallStat[];
	resourceReads?: AuditLogResourceReadStat[];
	promptReads?: AuditLogPromptReadStat[];
}

export interface AuditLogUsageStats {
	items: AuthLogUsageStatItem[];
	timeStart: string;
	timeEnd: string;
	totalCalls: number;
	uniqueUsers: number;
}

export type AuditLogFilters = {
	userId?: string | null;
	mcpServerCatalogEntryName?: string | null;
	mcpServerDisplayName?: string | null;
	client?: string | null;
	callType?: string | null; // tools/call, resources/read, prompts/get
	sessionId?: string | null;
	startTime?: string | null; // RFC3339 format (e.g., "2024-01-01T00:00:00Z"
	endTime?: string | null;
	limit?: number | null;
	offset?: number | null;
	sortBy?: string | null; // Field to sort by (e.g., "created_at", "user_id", "call_type")
	sortOrder?: string | null; // Sort order: "asc" or "desc"
};

export type AuditLogURLFilters = {
	user_id?: string | null;
	mcp_server_catalog_entry_name?: string | null;
	mcp_server_display_name?: string | null;
	mcp_id?: string | null;
	call_identifier?: string | null;
	client_name?: string | null;
	client_version?: string | null;
	client_ip?: string | null;
	call_type?: string | null; // tools/call, resources/read, prompts/get
	session_id?: string | null;
	start_time?: string | null; // RFC3339 format (e.g., "2024-01-01T00:00:00Z"
	end_time?: string | null;
	limit?: number | null;
	offset?: number | null;
	query?: string | null;
	response_status?: string | null;
};

export type UsageStatsFilters = {
	mcp_id?: string;
	user_ids?: string;
	mcp_server_display_names?: string;
	mcp_server_catalog_entry_names?: string;
	start_time?: string | null;
	end_time?: string | null;
};

export interface K8sServerEvent {
	action: string;
	count: number;
	eventType: string;
	message: string;
	reason: string;
	time: string;
}

export interface K8sServerDetail {
	deploymentName: string;
	events: K8sServerEvent[];
	isAvailable: boolean;
	lastRestart: string;
	namespace: string;
	readyReplicas: number;
	replicas: number;
}

export interface K8sServerLog {
	message: string;
}

export interface BaseAgent extends Project {
	allowedModels?: string[];
	allowedModelProviders?: string[];
	default?: boolean;
	model?: string; // default model
}

export interface MCPFilterManifest {
	name?: string;
	resources?: MCPFilterResource[];
	url: string;
	secret?: string;
	selectors?: MCPFilterWebhookSelector[];
	disabled?: boolean;
}

export interface MCPFilterResource {
	type: 'mcpServerCatalogEntry' | 'mcpServer' | 'selector' | 'mcpCatalog';
	id: string;
}

export interface MCPFilterWebhookSelector {
	method?: string;
	identifiers?: string[];
}

export interface MCPFilter extends MCPFilterManifest {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type: string;
	hasSecret: boolean;
}
