/**
 * Shared API/domain types for the user-facing client.
 *
 * **When editing this file** (including LLM-assisted changes), keep these conventions:
 *
 * 1. **Group by related types** — Organize exports into domain sections with a `// Section name`
 *    header (e.g. `// MCP servers`, `// Audit logs`). Keep related interfaces, types, and enums
 *    together; do not scatter a domain across the file.
 *
 * 2. **Sort alphabetically** —
 *    - Section headers: A–Z by section name.
 *    - Exports within a section: A–Z by symbol name (`export interface`, `export type`, `export enum`).
 *    - Properties on each interface/type: A–Z by field name (unless order is semantically required).
 *
 * 3. **New sections** — Add a section header, place it in alphabetical order among other sections,
 *    and keep all types for that domain inside it.
 */

// Access control rules

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

// App notifications

export type BannerType = 'info' | 'warning';
export interface AppNotification {
	banner?: {
		dismissible?: boolean;
		type?: BannerType;
		enabled?: boolean;
		text?: string;
		resetDismissed?: boolean;
	};
	updated?: string;
}

// App preferences

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

// Audit logs

export type McpAuditLogClient = {
	name: string;
	version: string;
};
export type McpAuditLogSourceType = 'mcp' | 'local_agent_tool_call';
export type McpLocalAgentToolCallAuditLogFields = {
	agentProvider: 'claude_code' | 'codex' | 'vscode' | 'cursor' | string;
	agentVersion?: string;
	cliName?: string;
	cliVersion: string;
	status: 'denied' | 'succeeded' | 'failed' | 'timeout' | string;
	failureType?: string;
	observedAt: string;
	startedAt?: string;
	idempotencyKey: string;
	toolUseID?: string;
	sessionID?: string;
	turnID?: string;
	toolName: string;
	toolKind?: string;
	mcpServerHint?: string;
	mcpToolName?: string;
	obotAuditCorrelationID?: string;
	model?: string;
	modelID?: string;
	permissionMode?: string;
	durationMs?: number;
	error?: string;
	deviceID?: string;
	hostname?: string;
	os?: string;
	arch?: string;
	localUsername?: string;
	reportedUserEmail?: string;
	identityStatus: 'authenticated_user' | 'anonymous_device' | 'unresolved' | string;
	cwd?: string;
	gitRepoRoot?: string;
	gitRemoteURLs?: string[];
	gitBranch?: string;
	gitCommitSHA?: string;
	transcriptPath?: string;
	toolInput: unknown;
	toolOutput: unknown;
	rawHookPayload: unknown;
};
export type McpAuditLogFields = {
	apiKey?: string;
	userAgent?: string;
	mcpServerDisplayName: string;
	mcpServerCatalogEntryName?: string;
	mcpID?: string;
	powerUserWorkspaceID?: string;
	client: McpAuditLogClient;
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
};
export interface McpAuditLog {
	id: string;
	createdAt: string;
	sourceType?: McpAuditLogSourceType;
	userID: string;
	clientIP: string;
	mcpFields?: McpAuditLogFields;
	localAgentToolCallFields?: McpLocalAgentToolCallAuditLogFields;
}
export interface McpAuditLogToolCallStatItem {
	createdAt: string;
	userID: string;
	processingTimeMs: number;
	responseStatus: number;
	error: string;
}
export interface McpAuditLogToolCallStat {
	toolName: string;
	callCount: number;
	items?: McpAuditLogToolCallStatItem[];
}
export interface McpAuditLogResourceReadStat {
	resourceUri: string;
	readCount: number;
}
export interface McpAuditLogPromptReadStat {
	promptName: string;
	readCount: number;
}
export interface McpAuthLogUsageStatItem {
	mcpID: string;
	mcpServerInstanceName: string;
	mcpServerName: string;
	mcpServerDisplayName: string;
	toolCalls?: McpAuditLogToolCallStat[];
	resourceReads?: McpAuditLogResourceReadStat[];
	promptReads?: McpAuditLogPromptReadStat[];
}
export interface McpAuditLogUsageStats {
	items: McpAuthLogUsageStatItem[];
	timeStart: string;
	timeEnd: string;
	totalCalls: number;
	uniqueUsers: number;
}
export type McpAuditLogURLFilters = {
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

export type McpAuditLogFilters = {
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
export type McpAuditLogUsageFilters = {
	mcp_id?: string;
	mcp_server_catalog_entry_names?: string;
	mcp_server_display_names?: string;
	user_ids?: string;
	start_time?: string | null;
	end_time?: string | null;
};
export type McpServerOrInstanceAuditLogStatsFilters = {
	start_time?: string;
	end_time?: string;
};
export type UsageStatsFilters = {
	mcp_id?: string;
	user_ids?: string;
	mcp_server_display_names?: string;
	mcp_server_catalog_entry_names?: string;
	start_time?: string | null;
	end_time?: string | null;
};

// Bootstrap

export interface BootstrapStatus {
	enabled: boolean;
	setupEnabled: boolean;
}

// Devices

// Device scans — payload shape matches apiclient/types/devicescan.go.
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
export interface DeviceScanFile {
	path: string;
	sizeBytes: number;
	oversized: boolean;
	content?: string;
}
export interface DeviceScanList {
	items: DeviceScan[] | null;
}
export type DeviceScanListFilters = {
	limit?: number;
	offset?: number;
	submittedBy?: string[];
	deviceId?: string[];
	groupByDevice?: boolean;
};
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
export interface DeviceScanResponse extends DeviceScanList {
	total: number;
	limit: number;
	offset: number;
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

// Files

export interface Files {
	items: File[];
}
export interface File {
	name: string;
}

// Images

export interface ImageResponse {
	imageUrl: string;
}

// K8s server

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

// Licensing
export interface LicenseEntitlementViolation {
	type: string;
	namespace: string;
	name: string;
	requiredEntitlements: string[];
	missingEntitlements: string[];
}

// MCP catalog servers

export interface MCPCatalogServer {
	id: string;
	alias?: string;
	userID: string;
	connectURL?: string;
	configured: boolean;
	catalogEntryID: string;
	missingRequiredEnvVars: string[];
	missingRequiredHeader?: string[];
	mcpCatalogID: string;
	created: string;
	deleted?: string;
	updated: string;
	type: string;
	mcpServerInstanceUserCount?: number;
	manifest: MCPServer;
	oauthMetadata?: OAuthMetadata;
	serverUserType: 'singleUser' | 'multiUser';
	needsUpdate?: boolean;
	needsK8sUpdate?: boolean;
	needsURL?: boolean;
	toolPreviewsLastGenerated?: string;
	lastUpdated?: string;
	powerUserWorkspaceID?: string;
	deploymentStatus?: string;
	compositeName?: string;
	canConnect?: boolean;
}
export interface OAuthMetadata {
	protectedResourceUrl?: string;
	authorizationServerUrl?: string;
	protectedResourceMetadata?: Record<string, unknown>;
	authorizationServerMetadata?: Record<string, unknown>;
	clientRegistration?: Record<string, unknown>;
	dynamicClientRegistration?: boolean;
	clientIdMetadataDocumentSupported?: boolean;
}
export interface MCPServerInstance {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
	multiUserConfig?: MultiUserConfig;
	configured: boolean;
	missingRequiredHeaders?: string[];
	userID: string;
	mcpServerID?: string;
	mcpCatalogID?: string;
	connectURL?: string;
}

// MCP runtime

export type Runtime = 'npx' | 'uvx' | 'containerized' | 'remote' | 'composite';
export interface MCPSubField {
	description: string;
	file?: boolean;
	dynamicFile?: boolean;
	key: string;
	name: string;
	required: boolean;
	sensitive: boolean;
	value?: string;
	prefix?: string;
	secretBinding?: MCPSecretBinding;
}
export interface MCPSecretBinding {
	name: string;
	key: string;
	adminAdded?: boolean;
}
export interface MCPAllowedSecretBindingTarget {
	name: string;
	keys: string[];
}
export interface UVXRuntimeConfig {
	package: string;
	command?: string;
	args?: string[];
	egressDomains?: string[];
	denyAllEgress?: boolean;
	startupTimeoutSeconds?: number;
}
export interface NPXRuntimeConfig {
	package: string;
	args?: string[];
	egressDomains?: string[];
	denyAllEgress?: boolean;
	startupTimeoutSeconds?: number;
}
export interface ContainerizedRuntimeConfig {
	image: string;
	port: number;
	path: string;
	healthzPath?: string;
	command?: string;
	args?: string[];
	egressDomains?: string[];
	denyAllEgress?: boolean;
	startupTimeoutSeconds?: number;
}
export interface MCPResourceRequests {
	cpu?: string;
	memory?: string;
}
export interface MCPResourceRequirements {
	requests?: MCPResourceRequests;
	limits?: MCPResourceRequests;
}
export interface RemoteRuntimeConfig {
	url: string;
	headers?: MCPSubField[];
	fixedURL?: string;
	hostname?: string;
	isTemplate?: boolean;
	urlTemplate?: string;
}
export interface RemoteCatalogConfig {
	fixedURL?: string;
	hostname?: string;
	urlTemplate?: string;
	headers?: MCPSubField[];
}
export type ResourceRuntimeConfig = MCPResourceRequirements;
export interface MultiUserConfig {
	userDefinedHeaders?: MCPSubField[];
}
export interface CompositeRuntimeConfig {
	componentServers: ComponentServer[];
}
export interface ComponentServer {
	catalogEntryID?: string;
	mcpServerID?: string;
	manifest?: MCPServer;
	toolOverrides?: ToolOverride[];
	toolPrefix?: string;
	disabled?: boolean;
}
export interface ToolOverride {
	name: string;
	/**
	 * Snapshot of the original tool description at the time the override was created.
	 * Used for display purposes only; the live description from the MCP server is
	 * still the source of truth unless an overrideDescription is provided.
	 */
	description?: string;
	/**
	 * Name exposed by the composite server. An empty or undefined value means
	 * the original tool name should be used.
	 */
	overrideName?: string;
	/**
	 * Optional description override. When empty or undefined, the live description
	 * from the MCP server should be used.
	 */
	overrideDescription?: string;
	/**
	 * Whether this tool is included in the composite server's allowlist.
	 */
	enabled?: boolean;
}

// MCP servers

export interface MCPServer {
	description?: string;
	shortDescription?: string;
	icon?: string;
	name?: string;
	env?: MCPSubField[];
	toolPreview?: MCPServerTool[];
	metadata?: {
		categories?: string;
		deprecated?: string;
	};

	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteRuntimeConfig;
	compositeConfig?: CompositeRuntimeConfig;
	multiUserConfig?: MultiUserConfig;
	resources?: MCPResourceRequirements;
}
export interface MCPServerTool {
	id: string;
	name: string;
	description?: string;
	metadata?: Record<string, string>;
	params?: Record<string, string>;
	credentials?: string[];
	enabled?: boolean;
	unsupported?: boolean;
}
export interface MCPServerPrompt {
	name: string;
	description: string;
	arguments?: {
		description: string;
		name: string;
		required: boolean;
	}[];
}
export interface McpServerResource {
	uri: string;
	name: string;
	mimeType: string;
}
export interface MCPInfo extends MCPServer {
	metadata?: {
		'allow-multiple'?: string;
		categories?: string;
	};
	repoURL?: string;
}
export interface MCP {
	id: string;
	created: string;
	manifest: MCPInfo;
	type: string;
}

// Models

export interface ModelProvider {
	id: string;
	name: string;
	description?: string;
	icon?: string;
	iconDark?: string;
	configured: boolean;
	modelsBackPopulated?: boolean;
	requiredConfigurationParameters?: {
		name: string;
		friendlyName?: string;
		description?: string;
		sensitive?: boolean;
		hidden?: boolean;
	}[];
	missingEntitlements?: string[];
	missingConfigurationParameters?: string[];
	created: string;
	optionalConfigurationParameters?: {
		name: string;
		friendlyName?: string;
		description?: string;
		sensitive?: boolean;
		hidden?: boolean;
	}[];
	image: string;
	port: number;
	path?: string;
	dialect?: string;
	type: 'modelprovider';
}
export interface ModelProviderList {
	items: ModelProvider[];
}
export interface Model {
	id: string;
	active: boolean;
	aliasAssigned: boolean;
	created: number;
	modelProvider: string;
	modelProviderName: string;
	name: string;
	displayName: string;
	targetModel: string;
	usage: string;
	icon?: string;
	iconDark?: string;
}
export const ModelAlias = {
	Llm: 'llm',
	LlmMini: 'llm-mini',
	TextEmbedding: 'text-embedding',
	ImageGeneration: 'image-generation',
	Vision: 'vision'
} as const;
export type ModelAlias = (typeof ModelAlias)[keyof typeof ModelAlias];
export interface DefaultModelAlias {
	alias: ModelAlias;
	model: string;
}

// Organization

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
export interface OrgGroup {
	id: string;
	name: string;
	iconURL?: string;
}

// Profile

export interface Profile {
	id: string;
	email: string;
	iconURL: string;
	role: number;
	effectiveRole: number;
	groups: string[];
	loaded?: boolean;
	hasAdminAccess?: () => boolean;
	isAdmin?: () => boolean;
	isAdminReadonly?: () => boolean;
	isBootstrapUser?: () => boolean;
	canImpersonate?: () => boolean;
	unauthorized?: boolean;
	username: string;
	currentAuthProvider?: string;
	expired?: boolean;
	created?: string;
	displayName?: string;
}

// Schedule

export interface Schedule {
	interval: string;
	hour: number;
	minute: number;
	day: number;
	weekday: number;
	timezone: string;
}

// Sites

export interface Sites {
	sites?: Site[];
	siteTool?: string;
}
export interface Site {
	site?: string;
	description?: string;
}

// Tool references

export interface ToolReference {
	id: string;
	name: string;
	description?: string;
	active: boolean;
	builtin: boolean;
	bundle?: boolean;
	bundleToolName?: string;
	created: string;
	credentials: string[];
	reference: string;
	resolved: boolean;
	revision: string;
	toolType: string;
	type: string;
	metadata?: {
		icon?: string;
		oauth?: string;
		category?: string;
	};
}
export interface ToolReferenceList {
	readonly?: boolean;
	items: ToolReference[];
}

// Version

export interface Version {
	sessionStore?: string;
	obot?: string;
	authEnabled?: boolean;
	enterprise?: boolean;
	licenseEntitlements?: string[];
	licenseEntitlementViolations?: LicenseEntitlementViolation[];
	missingLicenseEntitlements?: string[];
	upgradeAvailable?: boolean;
	engine?: 'docker' | 'kubernetes' | 'local';
	mcpNetworkPolicyEnabled?: boolean;
	mcpDefaultDenyAllEgress?: boolean;
	messagePoliciesEnabled?: boolean;
	agentsEnabled?: boolean;
	hideK8sDetails?: boolean;
	disableLegacyChat?: boolean;
}

// Workspaces

export type Workspace = {
	id: string;
	userID: string;
	created: string;
	role: number;
	type: string;
};
export type LaunchServerType = 'hosted' | 'multi' | 'remote' | 'composite';
