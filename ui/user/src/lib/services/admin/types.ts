import {
	type MCPServerTool,
	type MCPSecretBinding,
	type Project,
	type RemoteRuntimeConfig,
	type MultiUserConfig,
	type Runtime,
	type UVXRuntimeConfig,
	type NPXRuntimeConfig,
	type ContainerizedRuntimeConfig,
	type CompositeRuntimeConfig,
	type Task,
	type ToolOverride,
	type Schedule,
	ModelAlias
} from '../chat/types';

export interface MCPCatalogManifest {
	displayName: string;
	sourceURLs: string[];
	allowedUserIDs: string[];
	sourceURLCredentials?: Record<string, string>;
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
	urlTemplate?: string;
	hostname?: string;
	headers?: MCPCatalogEntryFieldManifest[];
	staticOAuthRequired?: boolean;
}

export interface CompositeCatalogConfig {
	componentServers: CatalogComponentServer[];
}

export interface CatalogComponentServer {
	catalogEntryID?: string;
	mcpServerID?: string;
	manifest?: MCPCatalogEntryServerManifest;
	toolOverrides?: ToolOverride[];
	toolPrefix?: string;
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
	compositeConfig?: CompositeCatalogConfig;
	multiUserConfig?: MultiUserConfig;
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
	needsUpdate?: boolean;
	canConnect?: boolean;
	needsK8sUpdate?: boolean;
	oauthCredentialConfigured?: boolean;
}

// Matches the backend compositeDeletionDependency struct used when preventing
// deletion of multi-user MCP servers that are still referenced by composites.
export interface MCPCompositeDeletionDependency {
	name: string;
	icon: string;
	mcpServerID?: string;
	catalogEntryID: string;
}

export interface MCPCatalogEntryFieldManifest {
	key: string;
	description: string;
	name: string;
	required: boolean;
	sensitive: boolean;
	value: string;
	file?: boolean;
	dynamicFile?: boolean;
	prefix?: string;
	secretBinding?: MCPSecretBinding;
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
	compositeConfig?: CompositeCatalogConfig; // For catalog entries
	compositeServerConfig?: CompositeRuntimeConfig; // For servers
	multiUserConfig?: MultiUserConfig; // For servers

	startupTimeoutSeconds?: number;
}

export interface MCPCatalogServerManifest {
	catalogEntryID?: string;
	manifest: Omit<MCPCatalogEntryServerManifest, 'remoteConfig'> & {
		remoteConfig?: RemoteRuntimeConfigAdmin;
		multiUserConfig?: MultiUserConfig;
	};
}

export interface MCPHeaderManifest {
	name: string;
	description: string;
	key: string;
	value: string;
	sensitive: boolean;
	required: boolean;
	prefix?: string;
	secretBinding?: MCPSecretBinding;
}

export interface MCPFilterRemoteRuntimeConfig {
	url: string;
	isTemplate?: boolean;
	urlTemplate?: string;
	hostname?: string;
	headers?: MCPHeaderManifest[];
	staticOAuthRequired?: boolean;
}

export interface MCPEnvManifest extends MCPHeaderManifest {
	file?: boolean;
	dynamicFile?: boolean;
}

export interface MCPFilterServerManifest {
	metadata?: Record<string, string>;
	name?: string;
	shortDescription?: string;
	description?: string;
	icon?: string;
	enabled?: boolean;
	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: MCPFilterRemoteRuntimeConfig;
	env?: MCPEnvManifest[];
}

export interface OrgUser {
	created: string;
	username: string;
	email: string;
	explicitRole: boolean;
	role: number;
	effectiveRole: number;
	groups: string[];
	iconURL: string;
	id: string;
	lastActiveDay?: string;
	displayName?: string;
	deletedAt?: string;
	originalEmail?: string;
	originalUsername?: string;
}

export interface TempUser {
	userId: number;
	username: string;
	email: string;
	role: number;
	groups: string[];
	iconUrl: string;
	authProviderName: string;
	authProviderNamespace: string;
	cachedAt: string;
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
	POWERUSER_PLUS: 64,
	POWERUSER: 128,
	USER_IMPERSONATION: 256
};

export const Group = {
	OWNER: 'owner',
	ADMIN: 'admin',
	POWERUSER_PLUS: 'power-user-plus',
	POWERUSER: 'power-user',
	USER: 'user',
	AUDITOR: 'auditor',
	USER_IMPERSONATION: 'user-impersonation'
};

export interface ProviderParameter {
	name: string;
	friendlyName?: string;
	description?: string;
	sensitive?: boolean;
	hidden?: boolean;
	multiline?: boolean;
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

export const NanobotModelAlias = {
	Llm: 'llm',
	LlmMini: 'llm-mini'
} as const;

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

export interface ModelResource {
	id: string;
}

export interface ModelAccessPolicyManifest {
	id?: string;
	displayName: string;
	subjects?: AccessControlRuleSubject[];
	models?: ModelResource[];
}

export interface ModelAccessPolicy extends Omit<ModelAccessPolicyManifest, 'id'> {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
}

export type PolicyDirection = 'user-message' | 'tool-calls' | 'both';

export const PolicyDirectionLabels: Record<PolicyDirection, string> = {
	'user-message': 'User Messages',
	'tool-calls': 'Tool Calls',
	both: 'Both'
};

export interface MessagePolicyManifest {
	id?: string;
	displayName: string;
	definition: string;
	direction: PolicyDirection;
	subjects?: AccessControlRuleSubject[];
}

export interface MessagePolicy extends Omit<MessagePolicyManifest, 'id'> {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
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
	apiKey?: string;
	userID: string;
	userAgent?: string;
	mcpServerInstanceName: string;
	mcpServerName: string;
	mcpServerDisplayName: string;
	mcpServerCatalogEntryName?: string;
	mcpID?: string;
	powerUserWorkspaceID?: string;
	client: AuditLogClient;
	clientIP: string;
	callType: string;
	callIdentifier?: string;
	responseStatus: number;
	processingTimeMs: number;
	requestHeaders?: Record<string, string | string[]>;
	requestMutated: boolean;
	requestBody?: unknown;
	mutatedRequestBody?: unknown;
	responseHeaders?: Record<string, string | string[]>;
	responseMutated: boolean;
	responseBody?: unknown;
	originalResponseBody?: unknown;
	webhookStatuses?: {
		type?: string;
		method?: string;
		name?: string;
		tool?: string;
		url?: string;
		status?: string;
		message?: string;
	}[];
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
	url?: string;
	toolName?: string;
	mcpServerManifest?: MCPFilterServerManifest;
	systemMCPServerCatalogEntryID?: string;
	secret?: string;
	selectors?: MCPFilterWebhookSelector[];
	disabled?: boolean;
	allowedToMutate?: boolean;
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
	configured: boolean;
	missingRequiredEnvVars?: string[];
}

export type MCPFilterInput = Omit<MCPFilter, 'id'> & { id?: string };

export interface AuditLogExportInput {
	name: string;
	bucket: string;
	startTime: string;
	endTime: string;
	filters: AuditLogExportFilters;
}

export interface AuditLogExport {
	id: string;
	name: string;
	bucket: string;
	keyPrefix?: string;
	storageProvider: string;
	startTime: string;
	endTime: string;
	state: 'pending' | 'running' | 'completed' | 'failed';
	error?: string;
	exportedRecords?: number;
	exportSize?: number;
	createdAt: string;
	completedAt?: string;
	filters: AuditLogExportFilterResponse;
}

export interface AuditLogExportFilterResponse {
	userIDs?: string[];
	mcpIDs?: string[];
	mcpServerDisplayNames?: string[];
	mcpServerCatalogEntryNames?: string[];
	callTypes?: string[];
	callIdentifiers?: string[];
	responseStatuses?: string[];
	sessionIDs?: string[];
	clientNames?: string[];
	clientVersions?: string[];
	clientIPs?: string[];
	query?: string;
}

export type AuditLogExportFilters = {
	userIDs?: string[];
	mcpIDs?: string[];
	mcpServerDisplayNames?: string[];
	mcpServerCatalogEntryNames?: string[];
	callTypes?: string[];
	callIdentifiers?: string[];
	responseStatuses?: string[];
	sessionIDs?: string[];
	clientNames?: string[];
	clientVersions?: string[];
	clientIPs?: string[];
	query?: string;
};

export interface ScheduledAuditLogExportInput {
	name: string;
	enabled: boolean;
	schedule: Schedule;
	bucket: string;
	retentionPeriodInDays: number;
	filters: AuditLogExportFilters;
}

export interface ScheduledAuditLogExport {
	id: string;
	name: string;
	enabled: boolean;
	schedule: Schedule;
	storageProvider: string;
	format: string;
	state: string;
	createdAt: string;
	lastRunAt: string;
	bucket: string;
	keyPrefix: string;
	retentionPeriodInDays: number;
	filters: AuditLogExportFilterResponse;
}

export interface StorageCredentials {
	provider: string;
	useWorkloadIdentity: boolean;
	s3Config?: {
		region: string;
		accessKeyID: string;
		secretAccessKey: string;
		sessionToken: string;
	};
	gcsConfig?: {
		serviceAccountJSON: string;
	};
	azureConfig?: {
		storageAccount: string;
		clientID: string;
		tenantID: string;
		clientSecret: string;
	};
	customS3Config?: {
		endpoint: string;
		region: string;
		accessKeyID: string;
		secretAccessKey: string;
	};
}

export interface K8sSettings {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type: string;
	affinity?: string;
	tolerations?: string;
	resources?: string;
	runtimeClassName?: string;
	storageClassName?: string;
	nanobotWorkspaceSize?: string;
	setViaHelm?: boolean;
}

export interface K8sSettingsManifest {
	affinity?: string;
	tolerations?: string;
	resources?: string;
	runtimeClassName?: string;
	storageClassName?: string;
	nanobotWorkspaceSize?: string;
}

export interface ServerK8sSettings {
	needsK8sUpdate: boolean;
	currentSettings: K8sSettings;
	deployedSettingsHash: string;
}
export type CompositeServerToolRow = {
	id: string;
	name: string;
	overrideName?: string;
	description?: string;
	overrideDescription?: string;
	enabled: boolean;
};

export class MCPCompositeDeletionDependencyError extends Error {
	constructor(
		message: string,
		public dependencies: MCPCompositeDeletionDependency[]
	) {
		super(message);
		this.name = 'MCPDeleteConflictError';
		this.dependencies = dependencies;
	}
}

export interface AppPreferencesManifest {
	logos?: {
		logoIcon?: string;
		logoIconError?: string;
		logoIconWarning?: string;
		logoDefault?: string;
		logoEnterprise?: string;
		logoChat?: string;
		darkLogoDefault?: string;
		darkLogoChat?: string;
		darkLogoEnterprise?: string;
	};
	theme?: {
		backgroundColor?: string;
		onBackgroundColor?: string;
		surface1Color?: string;
		surface2Color?: string;
		surface3Color?: string;
		secondaryColor?: string;
		successColor?: string;
		warningColor?: string;
		errorColor?: string;
		primaryColor?: string;
		onPrimaryColor?: string;
		onSuccessColor?: string;
		onWarningColor?: string;
		onErrorColor?: string;
		darkBackgroundColor?: string;
		darkOnBackgroundColor?: string;
		darkSurface1Color?: string;
		darkSurface2Color?: string;
		darkSurface3Color?: string;
		darkSecondaryColor?: string;
		darkSuccessColor?: string;
		darkWarningColor?: string;
		darkErrorColor?: string;
		darkPrimaryColor?: string;
		darkOnPrimaryColor?: string;
		darkOnSuccessColor?: string;
		darkOnWarningColor?: string;
		darkOnErrorColor?: string;
		fontFamily?: string;
	};
}

export interface AppPreferences {
	logos: {
		logoIcon: string;
		logoIconError: string;
		logoIconWarning: string;
		logoDefault: string;
		logoEnterprise: string;
		logoChat: string;
		darkLogoDefault: string;
		darkLogoChat: string;
		darkLogoEnterprise: string;
	};
	theme: {
		backgroundColor: string;
		onBackgroundColor: string;
		onPrimaryColor: string;
		onSuccessColor: string;
		onWarningColor: string;
		onErrorColor: string;
		surface1Color: string;
		surface2Color: string;
		surface3Color: string;
		secondaryColor: string;
		primaryColor: string;
		successColor: string;
		warningColor: string;
		errorColor: string;
		darkBackgroundColor: string;
		darkOnBackgroundColor: string;
		darkOnPrimaryColor: string;
		darkOnSuccessColor: string;
		darkOnWarningColor: string;
		darkOnErrorColor: string;
		darkSurface1Color: string;
		darkSurface2Color: string;
		darkSurface3Color: string;
		darkSecondaryColor: string;
		darkPrimaryColor: string;
		darkSuccessColor: string;
		darkWarningColor: string;
		darkErrorColor: string;
		fontFamily: string;
	};
}

export type GroupRoleAssignment = {
	groupName: string;
	role: number;
	description?: string;
};

export type GroupRoleAssignmentList = {
	items: GroupRoleAssignment[];
};

// MCP Capacity types
export type CapacitySource = 'resourceQuota' | 'deployments';

export interface MCPCapacityInfo {
	source: CapacitySource;
	cpuRequested?: string;
	cpuLimit?: string;
	memoryRequested?: string;
	memoryLimit?: string;
	activeDeployments: number;
	error?: string;
}

export interface MCPResourceRequests {
	cpu?: string;
	memory?: string;
}

// OAuth credential types for static OAuth configuration
export interface MCPServerOAuthCredentialRequest {
	clientID: string;
	clientSecret: string;
}

export interface MCPServerOAuthCredentialStatus {
	configured: boolean;
	clientID?: string;
}

export interface TokenUsage {
	userID?: string;
	runName?: string;
	model?: string;
	promptTokens: number;
	completionTokens: number;
	totalTokens: number;
	date: string;
}

export type TokenUsageWithCategory = TokenUsage & { category: string };

export interface TokenUsageTimeRange {
	start: Date | string;
	end: Date | string;
}

export interface TotalTokenUsage {
	promptTokens: number;
	completionTokens: number;
	totalTokens: number;
	personalToken?: boolean;
}

export interface TotalTokenUsageByUser extends TotalTokenUsage {
	userID: string;
}

export interface SkillRepository {
	id: string;
	created: string;
	deleted?: string;
	displayName: string;
	repoURL: string;
	ref: string;
	lastSyncTime?: string;
	isSyncing: boolean;
	syncError?: string;
	resolvedCommitSHA?: string;
	discoveredSkillCount: number;
}

export interface SkillRepositoryManifest {
	displayName: string;
	repoURL: string;
	ref: string;
}

export interface SkillAccessPolicyResource {
	type: 'skill' | 'skillRepository' | 'selector';
	id: string;
}

export interface SkillAccessPolicy {
	id: string;
	created: string;
	deleted?: string;
	displayName: string;
	subjects: AccessControlRuleSubject[];
	resources: SkillAccessPolicyResource[];
}

export interface SkillAccessPolicyManifest {
	displayName: string;
	subjects: AccessControlRuleSubject[];
	resources: SkillAccessPolicyResource[];
}

// Message Policy Violations
export interface MessagePolicyViolation {
	id: number;
	createdAt: string;
	userID: string;
	policyID: string;
	policyName: string;
	policyDefinition: string;
	direction: string;
	violationExplanation: string;
	blockedContent?: unknown;
	projectID: string;
	threadID: string;
}

export interface MessagePolicyViolationFilters {
	user_id?: string | null;
	policy_id?: string | null;
	direction?: string | null;
	project_id?: string | null;
	thread_id?: string | null;
	start_time?: string | null;
	end_time?: string | null;
	query?: string | null;
	limit?: number | null;
	offset?: number | null;
	sort_by?: string | null;
	sort_order?: string | null;
	time_group_by?: string | null;
}

export interface MessagePolicyViolationTimeBucket {
	time: string;
	category: string;
	count: number;
}

export interface MessagePolicyViolationPolicyCount {
	policyID: string;
	policyName: string;
	count: number;
}

export interface MessagePolicyViolationUserCount {
	userID: string;
	count: number;
}

export interface MessagePolicyViolationDirectionCounts {
	userMessage: number;
	toolCalls: number;
}

export interface MessagePolicyViolationStats {
	byTime: MessagePolicyViolationTimeBucket[];
	byPolicy: MessagePolicyViolationPolicyCount[];
	byUser: MessagePolicyViolationUserCount[];
	byDirection: MessagePolicyViolationDirectionCounts;
}

export interface SystemMCPServerManifest {
	metadata?: Record<string, string>;
	name: string;
	shortDescription: string;
	description: string;
	icon: string;
	enabled?: boolean;
	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteRuntimeConfig & {
		urlTemplate?: string;
		hostname?: string;
		staticOAuthRequired?: boolean;
	};
	env?: MCPEnvManifest[];
}

export interface SystemMCPServer {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type?: string;
	manifest: SystemMCPServerManifest;
	configured: boolean;
	missingRequiredEnvVars?: string[];
	missingRequiredHeaders?: string[];
	deploymentStatus?: string;
	deploymentAvailableReplicas?: number;
	deploymentReadyReplicas?: number;
	deploymentReplicas?: number;
	k8sSettingsHash?: string;
}

export interface RestartNanobotAgentDeploymentsFailure {
	serverID: string;
	error: string;
}

export interface RestartNanobotAgentDeploymentsResult {
	dryRun: boolean;
	totalNanobotAgentServers: number;
	targetedServerIDs: string[];
	restartedCount: number;
	restartedServerIDs: string[];
	failedCount: number;
	failed: RestartNanobotAgentDeploymentsFailure[];
}

// System MCP catalogs (admin API)

export interface SystemMCPCatalogManifest {
	displayName: string;
	sourceURLs: string[];
	sourceURLCredentials?: Record<string, string>;
}

export interface SystemMCPCatalog extends SystemMCPCatalogManifest {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type?: string;
	lastSynced?: string;
	syncErrors?: Record<string, string>;
	isSyncing?: boolean;
}

export type SystemMCPServerCatalogEntryServerType = 'filter';

export interface SystemMCPServerCatalogFilterConfig {
	toolName: string;
}

export interface SystemMCPServerCatalogEntryManifest {
	metadata?: Record<string, string>;
	name: string;
	shortDescription: string;
	description: string;
	icon: string;
	repoURL?: string;
	toolPreview?: MCPServerTool[];
	systemMCPServerType?: SystemMCPServerCatalogEntryServerType;
	filterConfig?: SystemMCPServerCatalogFilterConfig;
	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteCatalogConfigAdmin;
	env?: MCPEnvManifest[];
}

export interface SystemMCPServerCatalogEntry {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type?: string;
	manifest: SystemMCPServerCatalogEntryManifest;
	editable?: boolean;
	catalogName?: string;
	sourceURL?: string;
	lastUpdated?: string;
	toolPreviewsLastGenerated?: string;
	needsUpdate?: boolean;
	oauthCredentialConfigured?: boolean;
}

// Device scans — payload shape matches apiclient/types/devicescan.go.

export interface DeviceScanFile {
	path: string;
	sizeBytes: number;
	oversized: boolean;
	content?: string;
}

export interface DeviceScanMCPServer {
	id: number;
	client: string;
	projectPath?: string;
	file?: string;
	configHash?: string;
	envKeys: string[];
	headerKeys: string[];
	name: string;
	transport: string;
	command?: string;
	args?: string[];
	url?: string;
}

export interface DeviceScanSkill {
	id: number;
	client: string;
	projectPath?: string;
	file?: string;
	name: string;
	description?: string;
	files: string[];
	hasScripts: boolean;
	gitRemoteURL?: string;
}

export interface DeviceScanPlugin {
	id: number;
	client: string;
	projectPath?: string;
	configPath?: string;
	name: string;
	pluginType: string;
	version?: string;
	description?: string;
	author?: string;
	marketplace?: string;
	files: string[];
	enabled: boolean;
	hasMCPServers: boolean;
	hasSkills: boolean;
	hasRules: boolean;
	hasCommands: boolean;
	hasHooks: boolean;
}

export interface DeviceScanClient {
	name: string;
	version?: string;
	binaryPath?: string;
	installPath?: string;
	configPath?: string;
	hasMCPServers: boolean;
	hasSkills: boolean;
	hasPlugins: boolean;
}

export interface DeviceScan {
	id: number;
	receivedAt: string;
	submittedBy?: string;
	scannerVersion: string;
	scannedAt: string;
	deviceID: string;
	hostname: string;
	os: string;
	arch: string;
	username?: string;
	files: DeviceScanFile[];
	mcpServers: DeviceScanMCPServer[];
	skills: DeviceScanSkill[];
	plugins: DeviceScanPlugin[];
	clients: DeviceScanClient[];
}

export interface DeviceScanList {
	items: DeviceScan[] | null;
}

export interface DeviceScanResponse extends DeviceScanList {
	total: number;
	limit: number;
	offset: number;
}

export type DeviceScanListFilters = {
	limit?: number;
	offset?: number;
	submittedBy?: string[];
	deviceId?: string[];
	groupByDevice?: boolean;
};

export interface DeviceMCPServerStat {
	configHash: string;
	name: string;
	transport: string;
	command?: string;
	args?: string[];
	url?: string;
	deviceCount: number;
	userCount: number;
	clientCount: number;
	observationCount: number;
}

// DeviceMCPServerDetail extends the rollup row with EnvKeys and
// HeaderKeys, which are not in the hash and may vary per observation.

export interface DeviceMCPServerDetail extends DeviceMCPServerStat {
	envKeys: string[] | null;
	headerKeys: string[] | null;
}

export interface DeviceMCPServerOccurrence {
	deviceScanID: number;
	deviceID: string;
	client: string;
	scope: string;
	scannedAt: string;
	id: number;
}

export interface DeviceMCPServerOccurrenceList {
	items: DeviceMCPServerOccurrence[] | null;
}

export interface DeviceMCPServerOccurrenceResponse extends DeviceMCPServerOccurrenceList {
	total: number;
	limit: number;
	offset: number;
}

export interface DeviceClientStat {
	name: string;
	deviceCount: number;
	userCount: number;
	observationCount: number;
}

/** One skill row on a device client fleet summary (client match; excludes "multi"). */
export interface DeviceClientFleetSkill {
	name: string;
	description?: string;
	hasScripts: boolean;
	/** Number of file paths recorded for that skill observation. */
	files: number;
}

/** Rolls up latest-scan-per-device data for one canonical client name. */
export interface DeviceClientFleetSummary {
	name: string;
	users: string[] | null;
	skills: DeviceClientFleetSkill[] | null;
	mcpServers: DeviceMCPServerStat[] | null;
}

export interface DeviceClientFleetSummaryList {
	items: DeviceClientFleetSummary[] | null;
}

/** Returned by GET /api/devices/clients */
export interface DeviceClientFleetSummaryResponse extends DeviceClientFleetSummaryList {
	total: number;
	limit: number;
	offset: number;
}

export type DeviceClientListFilters = {
	/** Case-insensitive substring match on client name (server uses ILIKE on PostgreSQL). */
	name?: string;
	limit?: number;
	offset?: number;
};

export interface DeviceSkillStat {
	name: string;
	deviceCount: number;
	userCount: number;
	observationCount: number;
}

export interface DeviceSkillStatList {
	items: DeviceSkillStat[] | null;
}

export interface DeviceSkillStatResponse extends DeviceSkillStatList {
	total: number;
	limit: number;
	offset: number;
}

export type DeviceSkillSortKey = 'name' | 'device_count' | 'user_count' | 'observation_count';

export type DeviceSkillListFilters = {
	limit?: number;
	offset?: number;
	start?: string;
	end?: string;
	name?: string;
	sortBy?: DeviceSkillSortKey;
	sortOrder?: 'asc' | 'desc';
};

// DeviceSkillDetail is the per-skill drill-down. Metadata fields come
// from one canonical observation and are not guaranteed stable across
// observations sharing the same name.

export interface DeviceSkillDetail extends DeviceSkillStat {
	description?: string;
	hasScripts: boolean;
	gitRemoteURL?: string;
	files?: string[];
}

export interface DeviceSkillOccurrence {
	deviceScanID: number;
	deviceID: string;
	client: string;
	scope: string;
	projectPath?: string;
	scannedAt: string;
	id: number;
}

export interface DeviceSkillOccurrenceList {
	items: DeviceSkillOccurrence[] | null;
}

export interface DeviceSkillOccurrenceResponse extends DeviceSkillOccurrenceList {
	total: number;
	limit: number;
	offset: number;
}

// Dashboard rollup — single payload, full ranked breakdowns over each
// device's latest scan in the window. Returned by GET /api/devices/scan-stats.

export interface DeviceScanStats {
	timeStart: string;
	timeEnd: string;
	deviceCount: number;
	userCount: number;
	clients: DeviceClientStat[] | null;
	mcpServers: DeviceMCPServerStat[] | null;
	skills: DeviceSkillStat[] | null;
	scanTimestamps: string[] | null;
}
