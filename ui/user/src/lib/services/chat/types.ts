export interface Progress {
	runID?: string;
	parentRunID?: string;
	time: string;
	content: string;
	contentID?: string;
	input?: string;
	inputIsStepTemplateInput?: boolean;
	username?: string;
	stepTemplateInvoke?: StepTemplateInvoke;
	step?: Step;
	prompt?: Prompt;
	toolInput?: ToolInput;
	toolCall?: ToolCall;
	workflowCall?: WorkflowCall;
	waitingOnModel?: boolean;
	error?: string;
	threadID?: string;
	runComplete?: boolean;
	replayComplete?: boolean;
}

export interface Step {
	id: string;
}

type StepTemplateInvoke = {
	name?: string;
	description?: string;
	args?: { [key: string]: string };
	result?: string;
};

type Prompt = {
	id?: string;
	name?: string;
	description?: string;
	time: string;
	message?: string;
	fields?: PromptField[];
	sensitive?: boolean;
	metadata?: { [key: string]: string };
};

type PromptField = {
	name: string;
	description?: string;
	sensitive?: boolean;
	options?: string[];
};

type ToolInput = {
	name?: string;
	description?: string;
	input?: string;
	metadata?: { [key: string]: string };
};

type ToolCall = {
	name?: string;
	description?: string;
	input?: string;
	output?: string;
	taskID?: string;
	taskRunID?: string;
	metadata?: { [key: string]: string };
};

type WorkflowCall = {
	name?: string;
	description?: string;
	threadID?: string;
	workflowID?: string;
	input?: string;
};

export type CitationSource = { url?: string; content?: string };

export interface Message {
	runID: string;
	stepID?: string;
	parentRunID?: string;
	time?: Date;
	sent?: boolean;
	aborted?: boolean;
	icon?: string;
	tool?: boolean;
	toolCall?: ToolCall;
	username?: string;
	toolInput?: boolean;
	sourceName: string;
	sourceDescription?: string;
	done?: boolean;
	ignore?: boolean;
	message: string[];
	explain?: Explain;
	file?: MessageFile;
	oauthURL?: string;
	fields?: PromptField[];
	promptId?: string;
	contentID?: string;
	citations?: CitationSource[];
	// Optional notice to show beside a user's sent message as a tooltip
	userNotice?: string;
}

export interface InvokeInput {
	prompt?: string;
	explain?: Explain;
	improve?: Explain;
	changedFiles?: Record<string, string>;
}

export interface Explain {
	filename: string;
	selection: string;
}

export interface MessageFile {
	filename: string;
	content: string;
}

export interface ToolInfo {
	name: string;
	description: string;
	metadata: { [key: string]: string };
}

export interface InputMessage {
	prompt: string;
	type: string;
}

export interface Messages {
	lastRunID?: string;
	parentRunID?: string;
	messages: Message[];
	inProgress: boolean;
}

export interface Version {
	emailDomain?: string;
	dockerSupported?: boolean;
	sessionStore?: string;
	obot?: string;
	authEnabled?: boolean;
	enterprise?: boolean;
}

export interface Profile {
	id: string;
	email: string;
	iconURL: string;
	role: number;
	groups: string[];
	loaded?: boolean;
	isAdmin?: () => boolean;
	unauthorized?: boolean;
	username: string;
	currentAuthProvider?: string;
	expired?: boolean;
	created?: string;
	displayName?: string;
}

export interface Files {
	items: File[];
}

export interface File {
	name: string;
}

export interface KnowledgeFiles {
	items: KnowledgeFile[];
}

export interface KnowledgeFile {
	deleted?: string;
	fileName: string;
	state: string;
	error?: string;
}

export interface IngestionStatus {
	status: string;
}

export interface Assistants {
	items: Assistant[];
}

export interface AssistantIcons {
	icon?: string;
	iconDark?: string;
	collapsed?: string;
	collapsedDark?: string;
}

export interface Assistant {
	id: string;
	alias?: string;
	default?: boolean;
	name?: string;
	description?: string;
	current?: boolean;
	icons?: AssistantIcons;
	starterMessages?: string[];
	introductionMessage?: string;
	maxTools?: number;
	websiteKnowledge?: Sites;
	allowedModelProviders?: string[];
	tools?: string[];
	availableThreadTools?: string[];
	defaultThreadTools?: string[];
	allowedModels?: string[];
}

export type AssistantToolType = 'javascript' | 'python' | 'script' | 'container' | undefined;

export interface AssistantTool {
	id: string;
	name?: string;
	description?: string;
	icon?: string;
	enabled?: boolean;
	builtin?: boolean;
	capability?: boolean;
	toolType?: AssistantToolType;
	image?: string;
	instructions?: string;
	context?: string;
	params?: Record<string, string>;
}

export interface AssistantToolList {
	readonly?: boolean;
	items: AssistantTool[];
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

export type Runtime = 'npx' | 'uvx' | 'containerized' | 'remote';

export interface UVXRuntimeConfig {
	package: string;
	command?: string;
	args?: string[];
}

export interface NPXRuntimeConfig {
	package: string;
	args?: string[];
}

export interface ContainerizedRuntimeConfig {
	image: string;
	port: number;
	path: string;
	command?: string;
	args?: string[];
}

export interface RemoteRuntimeConfig {
	url: string;
	headers?: MCPSubField[];
}

export interface RemoteCatalogConfig {
	fixedURL?: string;
	hostname?: string;
	headers?: MCPSubField[];
}

export interface MCPSubField {
	description: string;
	file?: boolean;
	key: string;
	name: string;
	required: boolean;
	sensitive: boolean;
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

export interface McpServerGeneratedPrompt {
	description: string;
	messages: {
		content: {
			text: string;
			type: string;
			resource?: McpServerResource;
		};
		role: string;
	}[];
}

export interface McpServerResource {
	uri: string;
	name: string;
	mimeType: string;
}

export interface McpServerResourceContent {
	uri: string;
	mimeType: string;
	text?: string;
	blob?: string;
}

export interface MCPInfo extends MCPServer {
	metadata?: {
		'allow-multiple'?: string;
		categories?: string;
	};
	repoURL?: string;
}

export interface ProjectMCPList {
	items: ProjectMCP[];
}

export interface ProjectMCP {
	id: string;
	created: string;
	deleted?: boolean;
	type: string;
	userID: string;
	mcpID: string;
	alias?: string;
	name?: string;
	description?: string;
	icon?: string;
	configured?: boolean;
	needsUpdate?: boolean;
	needsURL?: boolean;
}

export interface Credential {
	toolName: string;
	icon: string;
}

export interface CredentialList {
	items: Credential[];
}

export interface TaskStep {
	id: string;
	step?: string;
	loop?: string[];
}

export interface Task {
	id: string;
	name?: string;
	description?: string;
	steps: TaskStep[];
	schedule?: Schedule;
	email?: object;
	webhook?: object;
	onDemand?: OnDemand;
	onSlackMessage?: object;
	onDiscordMessage?: object;
	alias?: string;
	managed?: boolean;
	projectID?: string;
}

export interface OnDemand {
	params?: Record<string, string>;
}

export interface Schedule {
	interval: string;
	hour: number;
	minute: number;
	day: number;
	weekday: number;
	timezone: string;
}

export interface TaskList {
	items: Task[];
}

export interface TaskRun {
	id: string;
	created: string;
	deleted?: string;
	taskID: string;
	threadID?: string;
	task: Task;
	startTime?: string;
	endTime?: string;
	input?: string;
	error?: string;
}

export interface TaskRunList {
	items: TaskRun[];
}

export interface Thread {
	id: string;
	created: string;
	deleted?: string;
	name: string;
	ready?: boolean;
	taskID?: string;
	taskRunID?: string;
	modelProvider?: string;
	model?: string;
}

export interface ThreadList {
	items: Thread[];
}

export interface Project {
	id: string;
	assistantID: string;
	created: string;
	deleted?: string;
	name: string;
	description?: string;
	icons?: AssistantIcons;
	starterMessages?: string[];
	introductionMessage?: string;
	prompt?: string;
	editor?: boolean;
	sourceProjectID?: string;
	tools?: string[];
	sharedTasks?: string[];
	websiteKnowledge?: Sites;
	capabilities: {
		onSlackMessage?: boolean;
		onDiscordMessage?: boolean;
		onEmail?: EmailManifest;
		onWebhook?: WebhookManifest;
	};
	defaultModelProvider?: string;
	defaultModel?: string;
	models?: Record<string, string[]>;
	userID: string;
	workflowNamesFromIntegration?: WorkflowNamesFromIntegration;
	templateUpgradeAvailable?: boolean;
	templateUpgradeInProgress?: boolean;
	templateLastUpgraded?: string;
	templatePublicID?: string;
}

export interface CreateProjectForm {
	name: string;
	description?: string;
	icons?: AssistantIcons;
	prompt?: string;
	editor?: boolean;
}

export interface WorkflowNamesFromIntegration {
	slackWorkflowName?: string;
	discordWorkflowName?: string;
	emailWorkflowName?: string;
	webhookWorkflowName?: string;
}

export interface WebhookManifest {
	headers: string[];
	secret: string;
	validationHeader: string;
}

export interface EmailManifest {
	allowedSenders?: string[];
}

export interface ProjectMember {
	userID: string;
	email: string;
	iconURL: string;
	isOwner: boolean;
}

export interface ProjectInvitation {
	code: string;
	project?: Project;
	status: 'pending' | 'accepted' | 'rejected' | 'expired';
	created?: string;
}

export interface ProjectList {
	items: Project[];
}

export interface ProjectShare {
	id: string;
	publicID: string;
	projectID: string;
	public: boolean;
	Users?: string[];
	name?: string;
	description?: string;
	icons?: AssistantIcons;
	featured?: boolean;
	tools?: string[];
	editor?: boolean;
}

export interface ProjectShareList {
	items: ProjectShare[];
}

export interface ProjectAuthorization {
	project?: Project;
	target: string;
	accepted?: boolean;
}

export interface ProjectAuthorizationList {
	items: ProjectAuthorization[];
}

export interface ProjectCredential {
	toolID: string;
	icon?: string;
	toolName?: string;
	exists?: boolean;
}

export interface ProjectCredentialList {
	items: ProjectCredential[];
}

export interface Sites {
	sites?: Site[];
	siteTool?: string;
}

export interface Site {
	site?: string;
	description?: string;
}

export interface SlackConfig {
	appId: string;
	clientId: string;
	clientSecret: string;
	signingSecret: string;
	appToken: string;
}

export interface SlackReceiver {
	appId: string;
	clientId: string;
}

export interface Memory {
	id: string;
	content: string;
	createdAt: string;
}

export interface MemoryList {
	items: Memory[];
}

export interface ThreadManifest {
	name: string;
	description?: string;
	icons?: AssistantIcons;
	introductionMessage?: string;
	starterMessages?: string[];
	websiteKnowledge?: Sites;
	tools?: string[];
	prompt?: string;
	sharedTasks?: string[];
}

export interface ProjectTemplate {
	id: string;
	created: string;
	deleted?: string;
	projectSnapshot: ThreadManifest;
	projectSnapshotStale?: boolean;
	projectSnapshotUpgradeInProgress?: boolean;
	projectSnapshotLastUpgraded?: string;
	mcpServers: string[];
	assistantID: string;
	projectID: string;
	publicID?: string;
	ready?: boolean;
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

export interface ChatModel {
	created: number;
	id: string;
	object: string;
	owned_by: string;
	root: string;
	parent: string;
	metadata: {
		usage: string;
	};
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

export interface ChatModelList {
	data: ChatModel[];
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
	needsUpdate?: boolean;
	needsURL?: boolean;
	toolPreviewsLastGenerated?: string;
	lastUpdated?: string;
	powerUserWorkspaceID?: string;
}

export interface MCPServerInstance {
	id: string;
	created: string;
	deleted?: string;
	links?: Record<string, string>;
	metadata?: Record<string, string>;
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
