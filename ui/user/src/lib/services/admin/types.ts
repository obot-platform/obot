import {
	type MCPServerTool,
	type MCPSecretBinding,
	type RemoteRuntimeConfig,
	type MultiUserConfig,
	type Runtime,
	type UVXRuntimeConfig,
	type NPXRuntimeConfig,
	type ContainerizedRuntimeConfig,
	type CompositeRuntimeConfig,
	type ToolOverride,
	type Schedule,
	ModelAlias,
	type AccessControlRuleSubject,
	type MCPResourceRequirements,
	type BannerType
} from '../user/types';

/**
 * Admin API/domain types for the admin client.
 *
 * **When editing this file** (including LLM-assisted changes), keep these conventions:
 *
 * 1. **Group by related types** — Organize exports into domain sections with a `// Section name`
 *    header (e.g. `// MCP catalog`, `// Audit log exports`). Keep related interfaces, types, and
 *    enums together; do not scatter a domain across the file.
 *
 * 2. **Sort alphabetically** —
 *    - Section headers: A–Z by section name.
 *    - Exports within a section: A–Z by symbol name (`export interface`, `export type`, `export enum`).
 *    - Properties on each interface/type: A–Z by field name (unless order is semantically required).
 *    - Named imports at the top: A–Z by symbol name.
 *
 * 3. **New sections** — Add a section header, place it in alphabetical order among other sections,
 *    and keep all types for that domain inside it.
 *
 * Prefer reusing or extending types from `../user/types` when the same shape exists there.
 */

// App notification

export interface AppNotificationManifest {
	banner: {
		dismissible?: boolean;
		type?: BannerType;
		enabled?: boolean;
		text?: string;
		resetDismissed?: boolean;
	};
}

// App preferences

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

// Audit log exports

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

// Auth and file scanner providers

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
	missingEntitlements?: string[];
	missingConfigurationParameters?: string[];
	optionalConfigurationParameters?: ProviderParameter[];
	requiredConfigurationParameters?: ProviderParameter[];
	icon?: string;
	iconDark?: string;
	id: string;
	link?: string;
	namespace?: string;
	image: string;
	port: number;
	path?: string;
}
export interface AuthProvider extends BaseProvider {
	type: 'authprovider';
}

// Devices
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
	sortBy?: DeviceClientSortKey;
	sortOrder?: 'asc' | 'desc';
};
export type DeviceClientSortKey = 'name' | 'mcp_server_count' | 'skill_count' | 'user_count';
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

// Group role assignments

export type GroupRoleAssignment = {
	groupName: string;
	role: number;
	description?: string;
};
export type GroupRoleAssignmentList = {
	items: GroupRoleAssignment[];
};

// Groups and roles

export const Group = {
	OWNER: 'owner',
	ADMIN: 'admin',
	POWERUSER_PLUS: 'power-user-plus',
	POWERUSER: 'power-user',
	USER: 'user',
	AUDITOR: 'auditor',
	USER_IMPERSONATION: 'user-impersonation'
};
export const Role = {
	BASIC: 4,
	OWNER: 8,
	ADMIN: 16,
	AUDITOR: 32,
	POWERUSER_PLUS: 64,
	POWERUSER: 128,
	USER_IMPERSONATION: 256
};

// Image pull secrets

export type ImagePullSecretType = 'basic' | 'ecr';
export interface ImagePullSecretCapability {
	available: boolean;
	reason?: string;
	issuerURL?: string;
	subject?: string;
	audience?: string;
}
export interface ImagePullSecret {
	id: string;
	created?: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	type?: string;
	manifest: ImagePullSecretManifest;
	status?: ImagePullSecretStatus;
}
export interface ImagePullSecretManifest {
	enabled: boolean;
	type?: ImagePullSecretType;
	displayName?: string;
	basic?: BasicImagePullSecretConfig;
	ecr?: ECRImagePullSecretConfig;
}
export interface ImagePullSecretStatus {
	passwordConfigured?: boolean;
	subject?: string;
	trustPolicyJSON?: string;
	ecrPolicyJSON?: string;
	lastReconciledTime?: string;
	lastSuccessTime?: string;
	lastError?: string;
	tokenExpiresAt?: string;
	registryEndpoints?: string[];
}
export interface BasicImagePullSecretConfig {
	server?: string;
	username?: string;
	password?: string;
}
export interface ECRImagePullSecretConfig {
	roleARN?: string;
	region?: string;
	issuerURL?: string;
	audience?: string;
	refreshSchedule?: string;
}
export interface ImagePullSecretTestRequest {
	image?: string;
}
export interface ImagePullSecretTestResponse {
	success: boolean;
	message?: string;
}
export interface ImagePullSecretRefreshResponse {
	message?: string;
}

// K8s settings

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

export interface AppK8sSettings {
	affinity?: string;
	tolerations?: string;
	resources?: string;
	runtimeClassName?: string;
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

// Licensing
export interface License {
	licenseKey: string;
	source: string;
	locked: boolean;
	enterprise: boolean;
	entitlements: string[] | null;
	manualCheckAvailableAt?: string;
}

export interface LicenseManifest {
	licenseKey: string;
}

// MCP capacity

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

// MCP catalog

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
	serverUserType: 'singleUser' | 'multiUser';
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteCatalogConfigAdmin;
	compositeConfig?: CompositeCatalogConfig;
	multiUserConfig?: MultiUserConfig;
	resources?: MCPResourceRequirements;
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
	connectURL?: string;
}

// Matches the backend compositeDeletionDependency struct used when preventing
// deletion of multi-user MCP servers that are still referenced by composites.
export interface MCPCompositeDeletionDependency {
	name: string;
	icon: string;
	mcpServerID?: string;
	catalogEntryID: string;
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
	serverUserType: 'singleUser' | 'multiUser';
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
	resources?: MCPResourceRequirements;

	startupTimeoutSeconds?: number;
}
export interface MCPCatalogServerManifest {
	catalogEntryID?: string;
	manifest: Omit<MCPCatalogEntryServerManifest, 'remoteConfig' | 'serverUserType'> & {
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

// MCP filters

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

// MCP OAuth

export interface MCPServerOAuthCredentialRequest {
	clientID: string;
	clientSecret: string;
}
export interface MCPServerOAuthCredentialStatus {
	configured: boolean;
	clientID?: string;
}

/** Subset of RFC 7591 / Obot OAuth client registration response used by the MCP OAuth debugger. */
export interface OAuthClient {
	client_id?: string;
	client_secret?: string;
	scope?: string;
	token_endpoint_auth_method?: string;
	redirect_uris?: string[];
	client_name?: string;
	registration_client_uri?: string;
	registration_access_token?: string;
}
export interface OAuthDebuggerRegisterClientResponse {
	state: string;
	client: OAuthClient;
	clientIdMetadataDocument?: boolean;
}
export interface OAuthDebuggerAuthorizationURLRequest {
	state: string;
}
export interface OAuthDebuggerAuthorizationURL {
	oauthURL: string;
	state?: string;
}
export interface OAuthDebuggerTokenRequest {
	code: string;
	state: string;
}
export interface OAuthToken {
	access_token: string;
	refresh_token: string;
	expires_in: number;
	token_type: string;
}

// Message policies

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

// Message policy violations

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

// Model access policies

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

// Models

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

// Setup

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

// Skills

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

// Storage credentials

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

// System MCP catalogs

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

// System MCP servers

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

// Token usage

export interface TokenUsage {
	date: string;
	userID?: string;
	model?: string;
	inputTokens: number;
	cacheReadTokens: number;
	cacheWriteTokens: number;
	outputTokens: number;
	thinkingTokens: number;
	totalTokens: number;
	inputSpend: number;
	cacheReadSpend: number;
	cacheWriteSpend: number;
	outputSpend: number;
	totalSpend: number;
}
export type TokenUsageWithCategory = TokenUsage & { category: string };
export interface TokenUsageTimeRange {
	start: Date | string;
	end: Date | string;
}
export interface TotalTokenUsage {
	totalTokens: number;
	inputTokens?: number;
	cacheReadTokens?: number;
	cacheWriteTokens?: number;
	outputTokens?: number;
	thinkingTokens?: number;
	inputSpend?: number;
	cacheReadSpend?: number;
	cacheWriteSpend?: number;
	outputSpend?: number;
	totalSpend?: number;
}
export interface TotalTokenUsageByUser extends TotalTokenUsage {
	userID: string;
}
