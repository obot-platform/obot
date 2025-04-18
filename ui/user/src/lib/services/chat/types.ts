export interface Progress {
	runID?: string;
	parentRunID?: string;
	time: string;
	content: string;
	contentID?: string;
	input?: string;
	inputIsStepTemplateInput?: boolean;
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
	messages: Message[];
	inProgress: boolean;
}

export interface Version {
	emailDomain?: string;
	dockerSupported?: boolean;
	sessionStore?: string;
}

export interface Profile {
	email: string;
	iconURL: string;
	role: number;
	loaded?: boolean;
	isAdmin?: () => boolean;
	getDisplayName?: () => string;
	unauthorized?: boolean;
	username: string;
	currentAuthProvider?: string;
	expired?: boolean;
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
		oath: string;
		category?: string;
	};
}

export interface ToolReferenceList {
	readonly?: boolean;
	items: ToolReference[];
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
	alias?: string;
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

export interface TableList {
	tables?: Table[];
}

export interface Table {
	name: string;
}

export interface Rows {
	columns: string[];
	rows: Record<string, unknown>[];
}

export interface Thread {
	id: string;
	created: string;
	deleted?: string;
	name: string;
	ready?: boolean;
	taskID?: string;
	taskRunID?: string;
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
	tools?: string[];
	sharedTasks?: string[];
	websiteKnowledge?: Sites;
	capabilities: {
		onSlackMessage?: boolean;
	};
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

export interface AuthProvider {
	configured: boolean;
	icon?: string;
	name: string;
	namespace: string;
	id: string;
}

export interface AuthProviderList {
	items: AuthProvider[];
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
