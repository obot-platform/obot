export interface Version {
	emailDomain?: string;
	dockerSupported?: boolean;
	sessionStore?: string;
	obot?: string;
	authEnabled?: boolean;
	enterprise?: boolean;
	upgradeAvailable?: boolean;
	engine?: 'docker' | 'kubernetes' | 'local';
	mcpNetworkPolicyEnabled?: boolean;
	mcpDefaultDenyAllEgress?: boolean;
	autonomousToolUseEnabled?: boolean;
	nanobotIntegration?: boolean;
	messagePoliciesEnabled?: boolean;
	disableLegacyChat?: boolean;
}

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
	autonomousToolUseEnabled: boolean;
}

export interface Files {
	items: File[];
}

export interface File {
	name: string;
}

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

export type Runtime = 'npx' | 'uvx' | 'containerized' | 'remote' | 'composite';

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
	command?: string;
	args?: string[];
	egressDomains?: string[];
	denyAllEgress?: boolean;
	startupTimeoutSeconds?: number;
}

export interface RemoteRuntimeConfig {
	url: string;
	headers?: MCPSubField[];
	fixedURL?: string;
	isTemplate?: boolean;
}

export interface RemoteCatalogConfig {
	fixedURL?: string;
	hostname?: string;
	headers?: MCPSubField[];
}

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
}

export interface MCP {
	id: string;
	created: string;
	manifest: MCPInfo;
	type: string;
}

export interface MCPServer {
	description?: string;
	icon?: string;
	name?: string;
	env?: MCPSubField[];
	toolPreview?: MCPServerTool[];
	metadata?: {
		categories?: string;
	};

	runtime: Runtime;
	uvxConfig?: UVXRuntimeConfig;
	npxConfig?: NPXRuntimeConfig;
	containerizedConfig?: ContainerizedRuntimeConfig;
	remoteConfig?: RemoteRuntimeConfig;
	compositeConfig?: CompositeRuntimeConfig;
	multiUserConfig?: MultiUserConfig;
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

export interface Schedule {
	interval: string;
	hour: number;
	minute: number;
	day: number;
	weekday: number;
	timezone: string;
}

export interface Sites {
	sites?: Site[];
	siteTool?: string;
}

export interface Site {
	site?: string;
	description?: string;
}

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
	missingConfigurationParameters?: string[];
	created: string;
	optionalConfigurationParameters?: {
		name: string;
		friendlyName?: string;
		description?: string;
		sensitive?: boolean;
		hidden?: boolean;
	}[];
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

export interface MCPCatalogServer {
	id: string;
	alias?: string;
	userID: string;
	connectURL?: string;
	configured: boolean;
	catalogEntryID: string;
	missingRequiredEnvVars: string[];
	missingRequiredHeaders: string[];
	mcpCatalogID: string;
	created: string;
	deleted?: string;
	updated: string;
	type: string;
	mcpServerInstanceUserCount?: number;
	manifest: MCPServer;
	oauthMetadata?: OAuthMetadata;
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

export type Workspace = {
	id: string;
	userID: string;
	created: string;
	role: number;
	type: string;
};

export type LaunchServerType = 'single' | 'multi' | 'remote' | 'composite';

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

export interface ImageResponse {
	imageUrl: string;
}
