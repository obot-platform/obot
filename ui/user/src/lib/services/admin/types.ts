import { m } from '$lib/i18n';
import {
	type MCPServerTool,
	type MCPSecretBinding,
	type MCPConfigurationOption,
	type RemoteRuntimeConfig,
	type MultiUserConfig,
	type Runtime,
	type UVXRuntimeConfig,
	type NPXRuntimeConfig,
	type ContainerizedRuntimeConfig,
	type OpenAPIRuntimeConfig,
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
		logoCommunity?: string;
		logoChat?: string;
		darkLogoDefault?: string;
		darkLogoChat?: string;
		darkLogoEnterprise?: string;
		darkLogoCommunity?: string;
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

export type AuditLogType = 'mcp' | 'llm';

export interface AuditLogExportInput {
	name: string;
	type?: AuditLogType;
	bucket: string;
	keyPrefix?: string;
	startTime: string;
	endTime: string;
	filters?: AuditLogExportFilters;
	llmFilters?: LLMAuditLogExportFilters;
}
export interface AuditLogExport {
	id: string;
	name: string;
	type: AuditLogType;
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
	filters?: AuditLogExportFilterResponse;
	llmFilters?: LLMAuditLogExportFilters;
}
export interface AuditLogExportFilterResponse {
	apiKeyIDs?: number[];
	sourceTypes?: string[];
	// Common cross-source filters (used when more than one source is selected).
	actors?: string[];
	operations?: string[];
	mcpServers?: string[];
	tools?: string[];
	outcomes?: string[];
	clients?: string[];
	// Single-source filters.
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
	agentProviders?: string[];
	statuses?: string[];
	toolNames?: string[];
	toolKinds?: string[];
	deviceIDs?: string[];
	query?: string;
}
export type AuditLogExportFilters = {
	apiKeyIDs?: number[];
	sourceTypes?: string[];
	// Common cross-source filters (used when more than one source is selected).
	actors?: string[];
	operations?: string[];
	mcpServers?: string[];
	tools?: string[];
	outcomes?: string[];
	clients?: string[];
	// Single-source filters.
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
	agentProviders?: string[];
	statuses?: string[];
	toolNames?: string[];
	toolKinds?: string[];
	deviceIDs?: string[];
	query?: string;
};
export interface ScheduledAuditLogExportInput {
	name: string;
	type?: AuditLogType;
	enabled: boolean;
	schedule: Schedule;
	bucket: string;
	keyPrefix?: string;
	retentionPeriodInDays: number;
	filters?: AuditLogExportFilters;
	llmFilters?: LLMAuditLogExportFilters;
}
export interface ScheduledAuditLogExport {
	id: string;
	name: string;
	type: AuditLogType;
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
	filters?: AuditLogExportFilterResponse;
	llmFilters?: LLMAuditLogExportFilters;
}

export type LLMAuditLogExportFilters = {
	apiKeyIDs?: number[];
	userIDs?: string[];
	modelProviders?: string[];
	targetModels?: string[];
	requestPaths?: string[];
	responseStatuses?: number[];
	outcomes?: string[];
	userAgents?: string[];
	clientSessionIDs?: string[];
	query?: string;
};

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
	// Settings saved as a replacement while another provider still serves logins.
	staged?: boolean;
	// The address that signed in through this staged provider to prove it works, and the address
	// that will hold Owner once the switch completes. Absent until a verification succeeds.
	verifiedEmail?: string;
	// A provisioned initial owner has not opened their setup link yet, so nobody can sign in.
	requiresActivation?: boolean;
	// The state of the provider's SCIM connection, absent when it has none. Deconfiguring the
	// provider deletes the connection. Only administrators see it.
	scimState?: SCIMConnectionState;
	// When the bearer token of the provider's SCIM connection stops being accepted. Absent while the
	// connection has no token, and for non-administrators.
	scimTokenExpiresAt?: string;
	// How the provider supports SCIM. Absent for providers that do not, and for non-administrators.
	scim?: AuthProviderSCIM;
}

export interface AuthProviderSCIM {
	// While the provider is configured or staged without a SCIM connection, providing these
	// parameters sets up directory synchronization, and leaving them empty sets up SCIM.
	directoryParameters: string[];
	// The parameter that holds the identity provider's issuer, such as the Okta Org URL.
	issuerParameter: string;
	// The issuer recorded when the provider's SCIM connection was created. SCIM bindings belong to
	// that organization.
	connectionIssuer?: string;
}

// A user of the built-in local auth provider. Passwords are never returned by the API.
export interface LocalAuthUser {
	id: string;
	orgUserId?: string;
	email: string;
	created: string;
	requirePasswordChange: boolean;
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

// Egress audit logs

export interface EgressAuditLog {
	actor: string;
	claudeAccount: string;
	claudeSession: string;
	createdAt: string;
	durationMs: number;
	environmentID: string;
	host: string;
	id: string;
	method: string;
	outcome: string;
	path: string;
	requestBody?: string;
	requestBytes: number;
	requestHeaders?: Record<string, string[]>;
	requestTruncated: boolean;
	responseBody?: string;
	responseBytes: number;
	responseHeaders?: Record<string, string[]>;
	responseStatus: number;
	responseTruncated: boolean;
	scheme: string;
	updatedAt: string;
	userID: string;
}
export type EgressAuditLogURLFilters = {
	claudeAccount?: string;
	claudeSession?: string;
	endTime?: string;
	environmentId?: string;
	host?: string;
	limit?: number;
	offset?: number;
	outcome?: string;
	startTime?: string;
	userId?: string;
};

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
	AUDITOR: 'auditor'
};
export const Role = {
	BASIC: 4,
	OWNER: 8,
	ADMIN: 16,
	AUDITOR: 32,
	POWERUSER_PLUS: 64,
	POWERUSER: 128
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

// Git credentials

export interface GitCredential {
	id: string;
	created?: string;
	deleted?: string;
	displayName: string;
	host: string;
	tokenConfigured: boolean;
	uses: GitCredentialUses;
}

export interface GitCredentialUses {
	skillRepositories: GitCredentialUse[];
	mcpCatalogs: GitCredentialUse[];
	systemMcpCatalogs: GitCredentialUse[];
}

export interface GitCredentialUse {
	id: string;
	displayName?: string;
}

export interface GitCredentialManifest {
	displayName: string;
	host: string;
	token?: string;
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
	maxCpuLimit?: string;
	maxCpuRequest?: string;
	maxMemoryLimit?: string;
	maxMemoryRequest?: string;
	maximumsSetViaHelm?: boolean;
	metadata?: Record<string, string>;
	type: string;
	affinity?: string;
	tolerations?: string;
	resources?: string;
	runtimeClassName?: string;
	setViaHelm?: boolean;
}

export interface K8sSettingsManifest {
	affinity?: string;
	maxCpuLimit?: string;
	maxCpuRequest?: string;
	maxMemoryLimit?: string;
	maxMemoryRequest?: string;
	tolerations?: string;
	resources?: string;
	runtimeClassName?: string;
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

export interface CommunityLicenseEnrollment {
	name: string;
	email: string;
	company?: string;
}

// LLM audit logs

// LLMAuditGuardrailFinding is what one guardrail policy did to an LLM request. It never
// includes the content that was found.
export interface LLMAuditGuardrailFinding {
	stage: 'input' | 'output';
	policyID: string;
	policyName: string;
	guardrailName?: string;
	guardrailType?: string;
	outcome: 'blocked' | 'redacted' | 'logged' | 'error';
	categories?: string[];
	error?: string;
}

export interface LLMAuditLog {
	apiKeyID?: number;
	apiKeyName?: string;
	apiKeyRevoked?: boolean;
	clientIP: string;
	clientSessionID: string;
	userAgent: string;
	createdAt: string;
	duration: number;
	error?: string;
	guardrailFindings?: LLMAuditGuardrailFinding[];
	id: string;
	inputTokens: number;
	modelID: string;
	modelProvider: string;
	outcome: string;
	outputTokens: number;
	// cacheReadTokens and cacheWriteTokens are part of inputTokens, reasoningTokens part of
	// outputTokens. spend is what the request cost in USD; absent when the model has no price.
	cacheReadTokens?: number;
	cacheWriteTokens?: number;
	reasoningTokens?: number;
	spend?: number;
	reasoningEffort: string;
	requestBody?: unknown;
	requestHeaders?: Record<string, string | string[]>;
	requestID: string;
	requestMethod: string;
	requestPath: string;
	responseBody?: unknown;
	responseHeaders?: Record<string, string | string[]>;
	responseID: string;
	responseStatus: number;
	// targetModel is the requested model; servedProvider and servedModel are the provider and
	// model that served the request, which routing rules and fallbacks can make differ.
	targetModel: string;
	servedProvider?: string;
	servedModel?: string;
	// providerKey names the gateway key the request was served with.
	providerKey?: string;
	fallback?: boolean;
	// claudeSubscription is true when the user's Claude subscription paid for the request, which
	// was then served with no provider key.
	claudeSubscription?: boolean;
	userID: string;
}

export type LLMAuditLogURLFilters = {
	api_key_id?: string | null;
	client_session_id?: string | null;
	end_time?: string | null;
	hide_models_requests?: string | null;
	limit?: number | null;
	model_provider?: string | null;
	offset?: number | null;
	outcome?: string | null;
	query?: string | null;
	request_path?: string | null;
	response_status?: string | null;
	served_provider?: string | null;
	sort_by?: string | null;
	sort_order?: string | null;
	start_time?: string | null;
	target_model?: string | null;
	user_agent?: string | null;
	user_id?: string | null;
};

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
	sourceURLGitCredentialIDs?: Record<string, string>;
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
	options?: MCPConfigurationOption[];
	required: boolean;
	sensitive: boolean;
	value: string;
	/** The value is stored by Obot and omitted from the manifest; reveal it to edit it. */
	static?: boolean;
	file?: boolean;
	dynamicFile?: boolean;
	interpolated?: boolean;
	prefix?: string;
	secretBinding?: MCPSecretBinding;
}
export interface RemoteRuntimeConfigAdmin {
	localhostCallbackEnabled?: boolean;
	localhostCallbackPath?: string;
	headers?: MCPCatalogEntryFieldManifest[];
	tunnelName?: string;
	url: string;
}
export interface RemoteCatalogConfigAdmin {
	localhostCallbackEnabled?: boolean;
	localhostCallbackPath?: string;
	fixedURL?: string;
	hostname?: string;
	staticOAuthRequired?: boolean;
	tunnelName?: string;
	urlTemplate?: string;
}
export interface LegacyRemoteCatalogConfigAdmin extends RemoteCatalogConfigAdmin {
	headers?: MCPCatalogEntryFieldManifest[];
}
export type MCPConfigUsage = 'env' | 'header' | 'file' | 'dynamicFile' | 'interpolated';
export type MCPConfig = Omit<
	MCPCatalogEntryFieldManifest,
	'file' | 'dynamicFile' | 'interpolated'
> & { usage: MCPConfigUsage; userAllowed?: boolean };
export interface MCPCatalogEntryServerManifest {
	entryKey?: string;
	upgradeNote?: string;
	minObotVersion?: string;
	maxObotVersion?: string;
	icon?: string;
	config?: MCPConfig[];
	repoURL?: string;
	name?: string;
	shortDescription?: string;
	description?: string;
	toolPreview?: MCPServerTool[];
	metadata?: {
		categories?: string;
		deprecated?: string;
		'allow-multiple'?: string;
	};

	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	openAPIConfig?: OpenAPIRuntimeConfig;
	remoteConfig?: RemoteCatalogConfigAdmin;
	resources?: MCPResourceRequirements;
	staticConfigurationRevision?: string;
}
export interface MCPCatalogEntry {
	id: string;
	created: string;
	deleted?: string;
	manifest: MCPCatalogEntryServerManifest;
	unsupportedTools?: string[];
	editable?: boolean;
	detached?: boolean;
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
	userID?: string;
}

export type MCPCatalogEntryFormData = Omit<MCPCatalogEntryServerManifest, 'metadata'> & {
	categories: string[];
	url?: string;
};

// New runtime-based form data structure
export interface RuntimeFormData {
	// Common fields
	name: string;
	shortDescription?: string;
	description: string;
	icon: string;
	categories: string[];
	metadata?: MCPCatalogEntryServerManifest['metadata'];
	serverUserType: 'singleUser' | 'multiUser';
	env: MCPCatalogEntryFieldManifest[];
	config?: MCPConfig[];

	// Runtime selection
	runtime: Runtime;

	// Runtime-specific configs (only one populated based on runtime)
	npxConfig?: NPXRuntimeConfig;
	uvxConfig?: UVXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	openAPIConfig?: OpenAPIRuntimeConfig;
	remoteConfig?: LegacyRemoteCatalogConfigAdmin; // Form state; flattened when saving catalog entries
	remoteServerConfig?: RemoteRuntimeConfigAdmin; // For servers
	multiUserConfig?: MultiUserConfig; // Form state; flattened into config when saving servers
	resources?: MCPResourceRequirements;

	startupTimeoutSeconds?: number;
}
export interface MCPCatalogServerManifest {
	catalogEntryID?: string;
	manifest: Omit<MCPCatalogEntryServerManifest, 'remoteConfig'> & {
		remoteConfig?: Omit<RemoteRuntimeConfigAdmin, 'headers'>;
	};
}
export interface MCPHeaderManifest {
	name: string;
	options?: MCPConfigurationOption[];
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
	removed?: boolean;
};

// MCP filters

export interface MCPFilterRemoteRuntimeConfig {
	localhostCallbackEnabled?: boolean;
	localhostCallbackPath?: string;
	url: string;
	isTemplate?: boolean;
	urlTemplate?: string;
	hostname?: string;
	staticOAuthRequired?: boolean;
}
export interface MCPEnvManifest extends MCPHeaderManifest {
	file?: boolean;
	dynamicFile?: boolean;
	interpolated?: boolean;
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
	config?: MCPConfig[];
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
	/** Set for guardrail filters, which run in-process with guardrails and have no server. */
	guardrail?: MCPGuardrailFilterConfig;
}
export type MCPGuardrailFilterAction = 'block' | 'redact';
export type MCPGuardrailFilterDirection = 'request' | 'response' | 'both';
export interface MCPGuardrailFilterSubject {
	type: 'user' | 'group' | 'everyone';
	id?: string;
}
export interface MCPGuardrailFilterConfig {
	guardrailIDs: string[];
	action: MCPGuardrailFilterAction;
	direction?: MCPGuardrailFilterDirection;
	failClosed?: boolean;
	samplingRate?: number;
	subjects?: MCPGuardrailFilterSubject[];
	condition?: string;
	blockMessage?: string;
}
/** A guardrail, as used by a guardrail filter. */
export interface MCPGuardrail {
	id: string;
	name: string;
	description?: string;
	type: string;
	error?: string;
}
/** A guardrail provider type, used to label guardrails and check redaction support. */
export interface MCPGuardrailProvider {
	type: string;
	label: string;
	description?: string;
	category?: string;
	redacts: boolean;
}
export type MCPFilterKind = 'guardrail' | 'webhook' | 'builtin' | 'mcpServer';
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
	clientSecret?: string;
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

// MCP tunnels

export interface MCPTunnel {
	created: string;
	deleted?: string;
	id: string;
	links?: Record<string, string>;
	manifest: MCPTunnelManifest;
	metadata?: Record<string, string>;
	token: string;
	type?: string;
}
export interface MCPTunnelManifest {
	allowedURLs?: string[];
	description?: string;
	displayName: string;
}
export interface TunnelConnection {
	name: string;
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

// Model proxy

export interface RegistrySettings {
	anonymousAccessEnabled: boolean;
}

export interface ModelProxySettings {
	enabled: boolean;
	url: string;
}

export interface ModelProxySettingsUpdateManifest {
	enabled: boolean;
}

export interface ModelProxyTokenUsage {
	max: number;
	used: number;
}

export interface ModelProxyUsage {
	input: ModelProxyTokenUsage;
	output: ModelProxyTokenUsage;
	resetAt: string;
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
	[ModelUsage.LLM]: m.core_model_usage_llm(),
	[ModelUsage.TextEmbedding]: m.core_model_usage_text_embedding(),
	[ModelUsage.ImageGeneration]: m.core_model_usage_image_generation(),
	[ModelUsage.Vision]: m.core_model_usage_vision(),
	[ModelUsage.Other]: m.core_model_usage_other(),
	[ModelUsage.Unknown]: m.core_unknown()
} as const;
export const ConfigurableModelAlias = {
	Llm: 'llm',
	LlmMini: 'llm-mini'
} as const;
export const ModelAliasLabels = {
	[ModelAlias.Llm]: m.core_model_usage_llm(),
	[ModelAlias.LlmMini]: m.core_model_usage_llm_fast(),
	[ModelAlias.TextEmbedding]: m.core_model_usage_text_embedding(),
	[ModelAlias.ImageGeneration]: m.core_model_usage_image_generation(),
	[ModelAlias.Vision]: m.core_model_usage_vision()
} as const;
export const ModelAliasToUsageMap = {
	[ModelAlias.Llm]: ModelUsage.LLM,
	[ModelAlias.LlmMini]: ModelUsage.LLM,
	[ModelAlias.TextEmbedding]: ModelUsage.TextEmbedding,
	[ModelAlias.ImageGeneration]: ModelUsage.ImageGeneration,
	[ModelAlias.Vision]: ModelUsage.Vision
} as const;

// Product analytics

export interface ProductTelemetryConsent {
	consent?: boolean;
}

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
	sourceURLCredentials?: Record<string, string>;
	gitCredentialID?: string;
}
export interface SkillRepositoryManifest {
	displayName: string;
	repoURL: string;
	ref: string;
	sourceURLCredentials?: Record<string, string>;
	gitCredentialID?: string;
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
	sourceURLGitCredentialIDs?: Record<string, string>;
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
	config?: MCPConfig[];
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
	config?: MCPConfig[];
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

// Token usage

export interface TokenUsage {
	date: string;
	userID?: string;
	model?: string;
	apiKeyID?: number;
	apiKeyName?: string;
	// provider is the provider that served the request.
	provider?: string;
	// outcome is success, error or canceled; empty on older records, which were all successes.
	outcome?: string;
	durationMs?: number;
	fallback?: boolean;
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
// Usage statistics over LLM gateway requests, from Obot's token usage records.
export interface TokenUsageStatsTotals {
	requests: number;
	// successes are the requests that succeeded; the others failed or were canceled.
	successes: number;
	// fallbacks are the requests a fallback served.
	fallbacks: number;
	avgDurationMs: number;
	inputTokens: number;
	outputTokens: number;
	totalTokens: number;
	totalSpend: number;
}

export interface TokenUsageStatsBucket extends TokenUsageStatsTotals {
	time: string;
}

export interface TokenUsageStatsProvider extends TokenUsageStatsTotals {
	provider: string;
}

export interface TokenUsageStatsModel extends TokenUsageStatsTotals {
	model: string;
	provider: string;
}

export interface TokenUsageStats {
	start: string;
	end: string;
	bucketSeconds: number;
	totals: TokenUsageStatsTotals;
	buckets: TokenUsageStatsBucket[] | null;
	providers: TokenUsageStatsProvider[] | null;
	topModelsByRequests: TokenUsageStatsModel[] | null;
	topModelsBySpend: TokenUsageStatsModel[] | null;
}

export interface TokenUsageStatsFilters {
	userIDs?: string[];
	models?: string[];
	apiKeyIDs?: string[];
}

export interface TotalTokenUsageByUser extends TotalTokenUsage {
	userID: string;
}

// MDM configurations — fleets that unattended devices enroll into. Saving
// values renders one downloadable artifact for every platform/OS target.
export interface MDMConfigurationArtifact {
	instructions: string;
	os: string;
	platform: string;
	slug: string;
}

export interface MDMConfiguration {
	assetDigest?: string;
	artifacts: MDMConfigurationArtifact[];
	createdAt: string;
	enforcementAllowlist?: EnforcementAllowlist;
	enforcementEnabled?: boolean;
	id: number;
	isDefault: boolean;
	// Version of the source bundle the saved artifacts were rendered from.
	obotSentryVersion?: string;
	values?: Record<string, unknown>;
}

export interface MDMConfigurationInput {
	assetDigest?: string;
	// Omitting the allowlist on creation lets the server seed its default policy.
	enforcementEnabled?: boolean;
	values?: Record<string, unknown>;
}

export interface MDMEnrollmentKey {
	createdAt: string;
	expiresAt?: string;
	id: number;
	lastUsedAt?: string;
	name?: string;
}

export interface MDMEnrollmentKeyCreateResponse extends MDMEnrollmentKey {
	enrollmentCredential: string;
}

export interface MDMDevice {
	deviceID: string;
	enrolledAt: string;
	hostname?: string;
	id: number;
	lastSeenAt?: string;
	latestScan?: MDMDeviceLatestScan;
	mdmConfigurationID: number;
	os?: string;
	osVersion?: string;
}

export interface MDMDeviceLatestScan {
	id: number;
	scannedAt: string;
	hostname?: string;
	username?: string;
	os?: string;
	arch?: string;
	scannerVersion?: string;
	submittedBy?: string;
	mcpServerCount: number;
	skillCount: number;
	pluginCount: number;
	clientCount: number;
}

// MDMAssetField is one property of the assets manifest's fields JSON
// Schema. readOnly properties are supplied server-side and hidden ones
// must not appear in forms; both annotations come from the manifest.
export interface MDMAssetField {
	default?: unknown;
	description?: string;
	enum?: (string | number)[];
	hidden?: boolean;
	maximum?: number;
	minimum?: number;
	readOnly?: boolean;
	title?: string;
	type?: string;
}

export interface MDMAssetPlatform {
	id: string;
	label: string;
	icon?: string;
}

// MDMAssetConfiguration is one platform/OS target offered by a bundle.
export interface MDMAssetConfiguration {
	assets: string[];
	description: string;
	instructions: string;
	os: string;
	osLabel: string;
	platform: string;
	suggestedName: string;
}

export interface MDMAssetFields {
	properties?: Record<string, MDMAssetField>;
	required?: string[];
}

export interface MDMAssetSource {
	id: string;
	source?: string;
	lastSyncTime?: string;
	isSyncing: boolean;
	syncError?: string;
	latestDigest?: string;
}

export interface MDMAsset {
	id: string;
	digest: string;
	schemaVersion: string;
	obotSentryVersion: string;
	configurations: MDMAssetConfiguration[];
	fields: MDMAssetFields;
	platforms: MDMAssetPlatform[];
}

export interface MDMAssetList {
	items: MDMAsset[] | null;
}

// Tool call enforcement — the allowlist attached to an MDMConfiguration decides
// which tool calls devices enrolled in that fleet may execute. Anything that
// does not positively match an allow rule is blocked.

export type AllowlistServerPackageSource = 'npm' | 'pypi';

export interface AllowlistServerPackage {
	name: string;
	source: AllowlistServerPackageSource;
	// Empty accepts any version.
	version?: string;
}

// Exactly one of url, package, hostname, or connector identifies the server.
export interface AllowlistServer {
	connector?: string;
	hostname?: string;
	package?: AllowlistServerPackage;
	// Empty allows every tool on the server.
	tools?: string[];
	url?: string;
}

export interface EnforcementAllowlist {
	allowAllBuiltinAgentMcpServers?: boolean;
	allowAllBuiltinAgentTools?: boolean;
	allowAllObotHostedMcpServers?: boolean;
	// Allows every call and short-circuits every other rule.
	allowEverything?: boolean;
	servers?: AllowlistServer[];
}

export interface MDMConfigurationEnforcementInput {
	enforcementAllowlist: EnforcementAllowlist;
	enforcementEnabled: boolean;
}

// EnforcementDecisionServer is the target the device resolved a tool call to.
export interface EnforcementDecisionServer {
	// Raw stdio command. Recorded for context only — it is not an allowlist
	// dimension, so a call identified only by its command cannot be allowlisted.
	command?: string;
	connector?: string;
	hostname?: string;
	package?: AllowlistServerPackage;
	url?: string;
}

export type EnforcementDecisionVerdict = 'allow' | 'deny';

export interface EnforcementDecisionEvent {
	agent?: string;
	clientIP?: string;
	createdAt: string;
	decision: EnforcementDecisionVerdict;
	deviceID?: string;
	id: string;
	kind?: string;
	mdmConfigurationID: number;
	obotHosted?: boolean;
	reason?: string;
	server?: EnforcementDecisionServer;
	serverName?: string;
	tool?: string;
	// The device could not establish what the call targeted, so it was blocked
	// before any rule was consulted. Always paired with a deny.
	unresolved?: boolean;
	unresolvedReason?: string;
}

// EnforcementDecisionAllowlistCheck is the server's answer to "would this
// recorded call be allowed if it were made now?".
export interface EnforcementDecisionAllowlistCheck {
	allowlistDecision: EnforcementDecisionVerdict;
	allowlistReason?: string;
	enforcementEnabled: boolean;
	id: string;
}

export type EnforcementDecisionURLFilters = {
	// actor is the enrolled device that produced the decision.
	actor?: string | null;
	agent?: string | null;
	decision?: string | null;
	end_time?: string | null;
	kind?: string | null;
	limit?: number | null;
	mdm_configuration_id?: string | null;
	offset?: number | null;
	query?: string | null;
	server?: string | null;
	sort_by?: string | null;
	sort_order?: string | null;
	start_time?: string | null;
	tool?: string | null;
};

// SCIM provisioning

export type SCIMConnectionState = 'connected' | 'enforced';

export interface SCIMConnection {
	id: string;
	adapterType: string;
	// 'scim_first' when the provider was configured without directory credentials, 'migrated' when
	// the connection replaced login-time directory synchronization.
	origin: 'scim_first' | 'migrated';
	authProviderNamespace: string;
	authProviderName: string;
	authProviderDisplayName: string;
	state: SCIMConnectionState;
	// The SCIM base URL to configure in the identity provider.
	baseURL: string;
	issuer?: string;
	enabledAt: string;
	enforcedAt?: string;
	// False until the first bearer token is issued.
	hasToken: boolean;
	tokenIssuedAt?: string;
	// When the token stops being accepted, a year after it was issued.
	tokenExpiresAt?: string;
	// True while the token replaced by the last rotation is still accepted.
	previousTokenAccepted: boolean;
	previousTokenExpiresAt?: string;
	// False while the connection's auth provider does not serve sign-ins, and SCIM requests fail.
	authProviderConfigured: boolean;
	// The bearer token. Only set in the response that issued it.
	token?: string;
}

export interface GroupReference {
	kind:
		| 'accessControlRule'
		| 'modelAccessPolicy'
		| 'skillAccessRule'
		| 'messagePolicy'
		| 'hostedAgentAccessRule'
		| 'groupRoleAssignment'
		| 'vmcpProfile'
		| 'mcpGuardrailFilter';
	id: string;
	displayName?: string;
	detail?: string;
}

export interface SCIMSetupGroup {
	id: string;
	// Empty for a referenced group ID that no group has.
	name: string;
	nativeID?: string;
	// Link to the group in the identity provider's admin console.
	consoleURL?: string;
	// Empty while the group is unbound.
	scimID?: string;
	references?: GroupReference[];
}

export interface SCIMSetupWarning {
	// 'duplicateName' and 'unreferencedNamesake' name groups whose name a pushed group cannot bind by.
	type: 'everyoneGroup' | 'missingGroup' | 'duplicateName' | 'unreferencedNamesake';
	message: string;
	groupID: string;
	groupName?: string;
	references?: GroupReference[];
}

export interface SCIMSetupUser {
	id: string;
	username?: string;
	email?: string;
	displayName?: string;
	status: 'active' | 'disabled';
	disabledReason?: string;
	// Empty while the user is unprovisioned.
	scimID?: string;
	active?: boolean;
	// True once the user has signed in through the auth provider.
	signedIn?: boolean;
}

export interface SCIMRequestFailure {
	time: string;
	method: string;
	resource: string;
	status: number;
	scimType?: string;
	detail?: string;
}

export interface SCIMPage<T> {
	items: T[];
	total: number;
}

export type SCIMGroupList = 'bound' | 'unboundReferenced' | 'unreferenced';

export interface SCIMConnectionReview {
	connection: SCIMConnection;
	provisionedUsers: SCIMPage<SCIMSetupUser>;
	unprovisionedUsers: SCIMPage<SCIMSetupUser>;
	boundGroups: SCIMPage<SCIMSetupGroup>;
	unboundReferencedGroups: SCIMPage<SCIMSetupGroup>;
	unreferencedGroups: SCIMPage<SCIMSetupGroup>;
	warnings: SCIMSetupWarning[];
	// Why the requesting user cannot enforce SCIM now, besides unboundReferencedGroups, which also
	// block it. Empty once SCIM is enforced.
	enforceBlockers: string[];
	activity: {
		lastRequestAt?: string;
		lastSuccessAt?: string;
		recentFailures: SCIMPage<SCIMRequestFailure>;
	};
}

export interface SCIMEnforceResult {
	connection: SCIMConnection;
	disabledUserCount: number;
	deletedGroupCount: number;
}

// A name that more than one referenced group has. A group pushed under it could bind to neither.
export interface SCIMDuplicateGroupName {
	name: string;
	groups: SCIMSetupGroup[];
}

// The configured auth provider that SCIM can be enabled for, which synchronizes its directory at
// sign-in, and what blocks enabling it.
export interface SCIMEnablePreview {
	// Absent when no auth provider is configured, or the configured one does not support SCIM.
	authProviderNamespace?: string;
	authProviderName?: string;
	authProviderDisplayName?: string;
	blockers: string[];
	duplicateGroupNames: SCIMDuplicateGroupName[];
}

export interface SCIMEnableResult {
	// Carries the token, which is shown only once.
	connection: SCIMConnection;
	deletedGroupCount: number;
	// Set when the unreferenced groups could not be deleted. SCIM is enabled regardless.
	deletionError?: string;
}

export interface SCIMGroupDeletionResult {
	deletedGroupCount: number;
}

// Group data left from an earlier configuration of an auth provider. It blocks configuring the
// provider without directory credentials until the provider's auth provider cleanup removes it.
export interface ResidualGroupData {
	groups: SCIMSetupGroup[];
	membershipCount: number;
}
