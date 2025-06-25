import type { AuthProvider, MCPCatalogEntry } from '../admin/types';
import { baseURL, doDelete, doGet, doPost, doPut, type Fetcher } from '../http';
import {
	type Assistant,
	type Assistants,
	type AssistantToolList,
	type ChatModelList,
	type Files,
	type InvokeInput,
	type KnowledgeFile,
	type KnowledgeFiles,
	type MCP,
	type MCPCatalogServer,
	type MCPServer,
	type McpServerGeneratedPrompt,
	type MCPServerInstance,
	type MCPServerPrompt,
	type McpServerResource,
	type McpServerResourceContent,
	type MCPServerTool,
	type Memory,
	type MemoryList,
	type ModelProviderList,
	type Profile,
	type Project,
	type ProjectAuthorizationList,
	type ProjectCredentialList,
	type ProjectInvitation,
	type ProjectList,
	type ProjectMCP,
	type ProjectMCPList,
	type ProjectMember,
	type ProjectShare,
	type ProjectShareList,
	type ProjectTemplate,
	type ProjectTemplateList,
	type ProjectTemplateManifest,
	type SlackConfig,
	type SlackReceiver,
	type Task,
	type TaskList,
	type TaskRun,
	type Thread,
	type ThreadList,
	type ToolReferenceList,
	type Version
} from './types';

type ItemsResponse<T> = { items: T[] | null };

export async function getProfile(opts?: { fetch?: Fetcher }): Promise<Profile> {
	const obj = (await doGet('/me', opts)) as Profile;
	obj.isAdmin = () => {
		return obj.role === 1;
	};
	obj.getDisplayName = () => {
		return obj?.currentAuthProvider === 'github-auth-provider' ? obj.username : obj?.email;
	};
	obj.loaded = true;
	return obj;
}

export async function deleteProfile() {
	return doDelete(`/me`);
}

export async function getVersion(): Promise<Version> {
	return (await doGet('/version')) as Version;
}

export async function getAssistant(id: string, opts?: { fetch?: Fetcher }): Promise<Assistant> {
	return (await doGet(`/assistants/${id}`, opts)) as Assistant;
}

export async function listAssistants(opts?: { fetch?: Fetcher }): Promise<Assistants> {
	const assistants = (await doGet(`/assistants`, opts)) as Assistants;
	if (!assistants.items) {
		assistants.items = [];
	}
	return assistants;
}

export async function deleteKnowledgeFile(
	assistantID: string,
	projectID: string,
	filename: string,
	opts?: {
		threadID?: string;
	}
) {
	let url = `/assistants/${assistantID}/projects/${projectID}/knowledge/${filename}`;
	if (opts?.threadID) {
		url = `/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/knowledge-files/${filename}`;
	}
	return doDelete(url);
}

export async function deleteFile(
	assistantID: string,
	projectID: string,
	filename: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	if (opts?.taskID && opts?.runID) {
		return doDelete(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`
		);
	} else if (opts?.threadID) {
		return doDelete(
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/file/${filename}`
		);
	}
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/file/${filename}`);
}

export async function download(
	assistantID: string,
	projectID: string,
	filename: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	let url = `/assistants/${assistantID}/projects/${projectID}/file/${filename}`;
	if (opts?.taskID && opts?.runID) {
		url = `/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`;
	} else if (opts?.threadID) {
		url = `/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/file/${filename}`;
	}
	url = baseURL + url;

	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
}

export async function saveFile(
	assistantID: string,
	projectID: string,
	file: File,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	if (opts?.taskID && opts?.runID) {
		return (await doPost(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/file/${file.name}`,
			file
		)) as Files;
	} else if (opts?.threadID) {
		return (await doPost(
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/file/${file.name}`,
			file
		)) as Files;
	}
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/file/${file.name}`,
		file
	)) as Files;
}

export async function saveContents(
	assistantID: string,
	projectID: string,
	filename: string,
	contents: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	if (opts?.taskID && opts?.runID) {
		return (await doPost(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`,
			contents
		)) as Files;
	} else if (opts?.threadID) {
		return (await doPost(
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/file/${filename}`,
			contents
		)) as Files;
	}
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/file/${filename}`,
		contents
	)) as Files;
}

export async function getFile(
	assistantID: string,
	projectID: string,
	filename: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
): Promise<Blob> {
	if (opts?.taskID && opts?.runID) {
		return (await doGet(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`,
			{
				blob: true
			}
		)) as Blob;
	} else if (opts?.threadID) {
		return (await doGet(
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/file/${filename}`,
			{
				blob: true
			}
		)) as Blob;
	}
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/file/${filename}`, {
		blob: true
	})) as Blob;
}

export async function uploadKnowledge(
	assistantID: string,
	projectID: string,
	file: File,
	opts?: {
		threadID?: string;
	}
): Promise<KnowledgeFile> {
	let url = `/assistants/${assistantID}/projects/${projectID}/knowledge/${file.name}`;
	if (opts?.threadID) {
		url = `/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/knowledge-files/${file.name}`;
	}
	return (await doPost(url, file)) as KnowledgeFile;
}

interface DeletedItems<T extends Deleted> {
	items: T[];
}

interface Deleted {
	deleted?: string;
}

function removedDeleted<V extends Deleted, T extends DeletedItems<V>>(items: T): T {
	items.items = items.items?.filter((item) => !item.deleted);
	return items;
}

export async function listKnowledgeFiles(
	assistantID: string,
	projectID: string,
	opts?: {
		threadID?: string;
	}
): Promise<KnowledgeFiles> {
	let url = `/assistants/${assistantID}/projects/${projectID}/knowledge`;
	if (opts?.threadID) {
		url = `/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/knowledge-files`;
	}
	const files = (await doGet(url)) as KnowledgeFiles;
	if (!files.items) {
		files.items = [];
	}
	return removedDeleted(files);
}

export async function listFiles(
	assistantID: string,
	projectID: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
): Promise<Files> {
	let files: Files;
	if (opts?.taskID && opts?.runID) {
		files = (await doGet(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/files`
		)) as Files;
	} else if (opts?.threadID) {
		files = (await doGet(
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts.threadID}/files`
		)) as Files;
	} else {
		files = (await doGet(`/assistants/${assistantID}/projects/${projectID}/files`)) as Files;
	}
	if (!files.items) {
		files.items = [];
	}
	return files;
}

function cleanInvokeInput(input: string | InvokeInput): InvokeInput | string {
	if (typeof input === 'string') {
		return input;
	}
	// This is just to make it pretty and send simple prompts if we can
	if (input.explain || input.improve) {
		return input;
	}
	if (input.changedFiles && Object.keys(input.changedFiles).length !== 0) {
		return input;
	}
	if (input.prompt) {
		return input.prompt;
	}
	return input;
}

export async function invoke(
	assistantID: string,
	projectID: string,
	threadID: string,
	msg: string | InvokeInput
) {
	msg = cleanInvokeInput(msg);
	await doPost(`/assistants/${assistantID}/projects/${projectID}/threads/${threadID}/invoke`, msg);
}

export async function abort(
	assistantID: string,
	projectID: string,
	opts?: { threadID?: string; taskID?: string; runID?: string }
) {
	if (opts?.taskID && opts?.runID) {
		return await doPost(
			`/assistants/${assistantID}/projects/${projectID}/tasks/${opts.taskID}/runs/${opts.runID}/abort`,
			{}
		);
	}
	await doPost(
		`/assistants/${assistantID}/projects/${projectID}/threads/${opts?.threadID}/abort`,
		{}
	);
}

export async function deleteProjectLocalCredential(
	assistantID: string,
	projectID: string,
	toolID: string
) {
	return doDelete(
		`/assistants/${assistantID}/projects/${projectID}/tools/${toolID}/local-deauthenticate`
	);
}

export async function listProjectLocalCredentials(
	assistantID: string,
	projectID: string
): Promise<ProjectCredentialList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/local-credentials`
	)) as ProjectCredentialList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listTools(
	assistantID: string,
	projectID: string,
	opts?: { fetch: Fetcher }
): Promise<AssistantToolList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tools`,
		opts
	)) as AssistantToolList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listAllTools(opts?: { fetch: Fetcher }): Promise<ToolReferenceList> {
	const list = (await doGet(`/tool-references?type=tool`, opts)) as ToolReferenceList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function getAssistantEnv(
	assistantID: string,
	projectID: string
): Promise<Record<string, string>> {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/env`)) as Record<
		string,
		string
	>;
}

export async function saveAssistantEnv(
	assistantID: string,
	projectID: string,
	env: Record<string, string>
): Promise<Record<string, string>> {
	return (await doPut(`/assistants/${assistantID}/projects/${projectID}/env`, env)) as Record<
		string,
		string
	>;
}

export async function saveTask(assistantID: string, projectID: string, task: Task): Promise<Task> {
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/tasks/${task.id}`,
		task
	)) as Task;
}

export async function runTask(
	assistantID: string,
	projectID: string,
	taskID: string,
	opts?: {
		stepID?: string;
		runID?: string;
		input?: string | object;
	}
): Promise<TaskRun> {
	let url = `/assistants/${assistantID}/projects/${projectID}/tasks/${taskID}/run`;
	if (opts?.runID) {
		url = `/assistants/${assistantID}/projects/${projectID}/tasks/${taskID}/runs/${opts.runID}/steps/${opts.stepID || '*'}/run`;
	} else if (opts?.stepID) {
		url += '?stepID=' + opts.stepID;
	}
	return (await doPost(url, opts?.input ?? {})) as TaskRun;
}

export async function deleteThread(assistantID: string, projectID: string, threadID: string) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/threads/${threadID}`);
}

export async function updateThread(
	assistantID: string,
	projectID: string,
	thread: Thread
): Promise<Thread> {
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/threads/${thread.id}`,
		thread
	)) as Thread;
}

export async function createThread(
	assistantID: string,
	projectID: string,
	body = {}
): Promise<Thread> {
	return (await doPost(`/assistants/${assistantID}/projects/${projectID}/threads`, body)) as Thread;
}

export async function getThread(
	assistantID: string,
	projectID: string,
	id: string
): Promise<Thread> {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/threads/${id}`)) as Thread;
}

export async function listThreads(assistantID: string, projectID: string): Promise<ThreadList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/threads`
	)) as ThreadList;
	if (!list.items) {
		list.items = [];
	}
	list.items.sort((a, b) => {
		return b.created.localeCompare(a.created);
	});
	list.items = list.items.filter((t) => !t.deleted);
	return list;
}

export function watchThreads(
	assistantID: string,
	projectID: string,
	onThread: (t: Thread) => void
): () => void {
	// This doesn't handle connection errors, should add that later
	const es = new EventSource(baseURL + `/assistants/${assistantID}/projects/${projectID}/threads`);
	es.onmessage = (e) => {
		const thread = JSON.parse(e.data) as Thread;
		onThread(thread);
	};
	return () => {
		es.close();
	};
}

export async function acceptPendingAuthorization(assistantID: string, projectID: string) {
	return doPut(`/assistants/${assistantID}/pending-authorizations/${projectID}`, {});
}

export async function rejectPendingAuthorization(assistantID: string, projectID: string) {
	return doDelete(`/assistants/${assistantID}/pending-authorizations/${projectID}`);
}

export async function listPendingAuthorizations(
	assistantID: string
): Promise<ProjectAuthorizationList> {
	const list = (await doGet(
		`/assistants/${assistantID}/pending-authorizations`
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function updateProjectTools(
	assistantID: string,
	projectID: string,
	tools: AssistantToolList
): Promise<AssistantToolList> {
	const list = (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/tools`,
		tools
	)) as AssistantToolList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function updateProjectAuthorizations(
	assistantID: string,
	projectID: string,
	authorizations: ProjectAuthorizationList
): Promise<ProjectAuthorizationList> {
	const list = (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/authorizations`,
		authorizations
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listProjectCredentials(
	assistantID: string,
	projectID: string
): Promise<ProjectCredentialList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/credentials`
	)) as ProjectCredentialList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listProjectAuthorizations(
	assistantID: string,
	id: string
): Promise<ProjectAuthorizationList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${id}/authorizations`
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function deleteProjectCredential(
	assistantID: string,
	projectID: string,
	toolID: string
) {
	return doDelete(
		`/assistants/${assistantID}/projects/${projectID}/tools/${toolID}/deauthenticate`
	);
}

export async function createProject(
	assistantID: string,
	opts?: {
		name?: string;
		description?: string;
		fetch?: Fetcher;
	}
): Promise<Project> {
	return (await doPost(
		`/assistants/${assistantID}/projects`,
		{
			name: opts?.name,
			description: opts?.description
		},
		{
			fetch: opts?.fetch
		}
	)) as Project;
}

export async function getProject(id: string, opts?: { fetch?: Fetcher }): Promise<Project> {
	return (await doGet(`/projects/${id}`, opts)) as Project;
}
export async function getProjectDefaultModel(
	assistantId: string,
	projectId: string,
	opts?: { fetch?: Fetcher }
): Promise<{ modelProvider: string; model: string }> {
	return doGet(`/assistants/${assistantId}/projects/${projectId}/default-model`, opts) as Promise<{
		modelProvider: string;
		model: string;
	}>;
}

export async function deleteProject(assistantID: string, id: string) {
	return doDelete(`/assistants/${assistantID}/projects/${id}`);
}

export async function updateProject(project: Project): Promise<Project> {
	const result = (await doPut(
		`/assistants/${project.assistantID}/projects/${project.id}`,
		project
	)) as Project;
	return result;
}

export async function listProjects(opts?: {
	editor?: boolean;
	fetch?: Fetcher;
}): Promise<ProjectList> {
	let url = '/projects';
	if (opts?.editor !== undefined) {
		url += `?editor=${opts.editor}`;
	}
	const list = (await doGet(url, opts)) as ProjectList;
	if (!list.items) {
		list.items = [];
	}
	list.items = list.items.filter((project) => !project.deleted);
	return list;
}

export function newMessageEventSource(
	assistantID: string,
	projectID: string,
	opts?: {
		authenticate?: {
			tools?: string[];
			local?: boolean;
		};
		threadID?: string;
		task?: {
			id: string;
		};
		runID?: string;
		follow?: boolean;
		history?: boolean;
	}
): EventSource {
	if (opts?.authenticate?.tools) {
		let url = `/assistants/${assistantID}/projects/${projectID}/tools/${opts.authenticate.tools.join(',')}/authenticate`;
		if (opts.authenticate.local) {
			url = `/assistants/${assistantID}/projects/${projectID}/tools/${opts.authenticate.tools.join(',')}/local-authenticate`;
		}
		return new EventSource(baseURL + url);
	}
	if (opts?.task?.id && opts?.runID) {
		const url = `/assistants/${assistantID}/projects/${projectID}/tasks/${opts.task.id}/runs/${opts.runID}/events`;
		return new EventSource(baseURL + `${url}`);
	}
	const queryParams = [];
	if (opts?.follow) {
		queryParams.push(`follow=${String(opts.follow)}`);
	}
	if (opts?.runID) {
		queryParams.push(`runID=${opts.runID}`);
	}
	if (opts?.history) {
		queryParams.push(`history=${String(opts.history)}`);
	}

	const queryString = queryParams.length > 0 ? `?${queryParams.join('&')}` : '';
	return new EventSource(
		baseURL +
			`/assistants/${assistantID}/projects/${projectID}/threads/${opts?.threadID}/events${queryString}`
	);
}

export async function listTasks(assistantID: string, projectID: string): Promise<TaskList> {
	const list = (await doGet(`/assistants/${assistantID}/projects/${projectID}/tasks`)) as TaskList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function createTask(
	assistantID: string,
	projectID: string,
	task?: Task
): Promise<Task> {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/tasks`,
		task ?? {
			steps: []
		}
	)) as Task;
}

export async function deleteTask(assistantID: string, projectID: string, id: string) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/tasks/${id}`);
}

export async function getTask(assistantID: string, projectID: string, id: string): Promise<Task> {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/tasks/${id}`)) as Task;
}

export async function getTaskRun(
	assistantID: string,
	projectID: string,
	taskID: string,
	runID: string
): Promise<TaskRun> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tasks/${taskID}/runs/${runID}`
	)) as TaskRun;
}

export async function deleteTaskRun(
	assistantID: string,
	projectID: string,
	id: string,
	runID: string
) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/tasks/${id}/runs/${runID}`);
}

export async function sendCredentials(id: string, credentials: Record<string, string>) {
	return await doPost('/prompt', { id, response: credentials });
}

export async function listAuthProviders(opts?: { fetch?: Fetcher }): Promise<AuthProvider[]> {
	const list = (await doGet('/auth-providers', opts)) as ItemsResponse<AuthProvider>;
	if (!list.items) {
		list.items = [];
	}
	return list.items.filter((provider) => provider.configured);
}

export async function setFeatured(assistantID: string, projectID: string, featured: boolean) {
	return (await doPut(`/assistants/${assistantID}/projects/${projectID}/featured`, {
		featured
	})) as ProjectShare;
}

export async function getProjectShare(
	assistantID: string,
	projectID: string
): Promise<ProjectShare> {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/share`)) as ProjectShare;
}

export async function deleteProjectShare(assistantID: string, projectID: string) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/share`);
}

export async function createProjectShare(assistantID: string, projectID: string) {
	return (await doPost(`/assistants/${assistantID}/projects/${projectID}/share`, {
		public: true
	})) as ProjectShare;
}

export async function createProjectFromShare(
	id: string,
	opts?: {
		fetch?: Fetcher;
		create: boolean;
	}
): Promise<Project> {
	if (opts?.create) {
		return (await doPost(`/shares/${id}?create`, {})) as Project;
	}
	return (await doPost(`/shares/${id}`, {}, opts)) as Project;
}

export async function listProjectShares(opts?: { fetch?: Fetcher }): Promise<ProjectShareList> {
	const list = (await doGet(`/shares`, opts)) as ProjectShareList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function getProjectShareByPublicID(
	publicID: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectShare> {
	return (await doGet(`/shares/${publicID}`, opts)) as ProjectShare;
}

export async function copyProject(assistantID: string, projectID: string): Promise<Project> {
	return (await doPost(`/assistants/${assistantID}/projects/${projectID}/copy`, {})) as Project;
}

export async function listProjectThreadTools(
	assistantID: string,
	projectID: string,
	threadID: string
): Promise<AssistantToolList> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/threads/${threadID}/tools`
	)) as AssistantToolList;
}

export async function updateProjectThreadTools(
	assistantID: string,
	projectID: string,
	threadID: string,
	tools: AssistantToolList
): Promise<AssistantToolList> {
	const list = (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/threads/${threadID}/tools`,
		tools
	)) as AssistantToolList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function configureProjectSlack(
	assistantID: string,
	projectID: string,
	config: SlackConfig,
	method: 'POST' | 'PUT' = 'POST'
) {
	if (method === 'POST') {
		return (await doPost(
			`/assistants/${assistantID}/projects/${projectID}/slack`,
			config
		)) as SlackReceiver;
	}
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/slack`,
		config
	)) as SlackReceiver;
}

export async function getProjectSlack(assistantID: string, projectID: string) {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/slack`, {
		dontLogErrors: true
	})) as SlackReceiver;
}

export async function disableProjectSlack(assistantID: string, projectID: string) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/slack`);
}

export async function getMemories(assistantID: string, projectID: string): Promise<MemoryList> {
	return doGet(`/assistants/${assistantID}/projects/${projectID}/memories`, {
		dontLogErrors: true
	}) as Promise<MemoryList>;
}

export async function deleteAllMemories(assistantID: string, projectID: string): Promise<void> {
	await doDelete(`/assistants/${assistantID}/projects/${projectID}/memories`);
}

export async function deleteMemory(
	assistantID: string,
	projectID: string,
	memoryID: string
): Promise<void> {
	await doDelete(`/assistants/${assistantID}/projects/${projectID}/memories/${memoryID}`);
}

export async function createMemory(
	assistantID: string,
	projectID: string,
	content: string
): Promise<Memory> {
	return doPost(`/assistants/${assistantID}/projects/${projectID}/memories`, {
		content
	}) as Promise<Memory>;
}

export async function updateMemory(
	assistantID: string,
	projectID: string,
	memoryID: string,
	content: string
): Promise<Memory> {
	return doPut(`/assistants/${assistantID}/projects/${projectID}/memories/${memoryID}`, {
		content
	}) as Promise<Memory>;
}

export async function listMCPs(opts?: { fetch?: Fetcher }): Promise<MCPCatalogEntry[]> {
	const response = (await doGet(
		'/all-mcp-catalogs/entries',
		opts
	)) as ItemsResponse<MCPCatalogEntry>;
	return response.items ?? [];
}

export async function getMCP(id: string, opts?: { fetch?: Fetcher }): Promise<MCP> {
	return (await doGet(`/all-mcp-catalogs/entries/${id}`, opts)) as MCP;
}

export async function listMCPCatalogServers(opts?: {
	fetch?: Fetcher;
}): Promise<MCPCatalogServer[]> {
	const response = (await doGet('/all-mcp-catalogs/servers', opts)) as {
		items: MCPCatalogServer[];
	};
	return response.items;
}

export async function listProjectMCPs(
	assistantID: string,
	projectID: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectMCP[]> {
	const response = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers`,
		opts
	)) as ProjectMCPList;
	return response.items ?? [];
}

export async function createProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerManifest?: MCPServer,
	catalogEntryID?: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectMCP> {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers`,
		catalogEntryID ? { catalogEntryID, ...(mcpServerManifest || {}) } : mcpServerManifest || {},
		opts
	)) as ProjectMCP;
}

export async function updateProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string,
	mcpServerManifest: MCPServer
) {
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}`,
		mcpServerManifest
	)) as ProjectMCP;
}

export async function deleteProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string
) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}`);
}

export async function revealProjectMCPEnvHeaders(
	assistantID: string,
	projectID: string,
	mcpServerId: string
) {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/reveal`,
		{},
		{
			dontLogErrors: true
		}
	)) as Record<string, string>;
}

export async function revealSharedProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string
) {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/reveal-shared`,
		{},
		{
			dontLogErrors: true
		}
	)) as Record<string, string>;
}

export async function configureProjectMCPEnvHeaders(
	assistantID: string,
	projectID: string,
	mcpServerId: string,
	envHeadersToConfigure: Record<string, string>
) {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/configure`,
		envHeadersToConfigure
	)) as ProjectMCP;
}

export async function deconfigureProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string
) {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/deconfigure`,
		{}
	);
}

export async function configureSharedProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string,
	envHeadersToConfigure: Record<string, string>
) {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/configure-shared`,
		envHeadersToConfigure
	);
}

export async function deconfigureSharedProjectMCP(
	assistantID: string,
	projectID: string,
	mcpServerId: string
) {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${mcpServerId}/deconfigure-shared`,
		{}
	);
}

export async function listProjectMCPServerTools(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string
): Promise<MCPServerTool[]> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/tools`,
		{
			dontLogErrors: true
		}
	)) as MCPServerTool[];
}

export async function configureProjectMcpServerTools(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string,
	toolIds?: string[]
) {
	// tools array sent are the enabled tools
	// send [] to disable all tools or [*] to enable all tools
	return doPut(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/tools`,
		toolIds ? toolIds : ['*']
	);
}

export async function listProjectThreadMcpServerTools(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string,
	threadID: string
): Promise<MCPServerTool[]> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/tools/${threadID}`,
		{
			dontLogErrors: true
		}
	)) as MCPServerTool[];
}

export async function configureProjectThreadMcpServerTools(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string,
	threadID: string,
	toolIds?: string[]
) {
	// tools array sent are the enabled tools
	// send [] to disable all tools or [*] to enable all tools
	return doPut(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/tools/${threadID}`,
		toolIds ? toolIds : ['*']
	);
}

export async function listProjectMcpServerPrompts(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string
): Promise<MCPServerPrompt[]> {
	const response = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/prompts`,
		{
			dontLogErrors: true
		}
	)) as MCPServerPrompt[];
	return response;
}

export async function generateProjectMcpServerPrompt(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string,
	promptName: string,
	promptParams?: Record<string, string>
) {
	const response = (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/prompts/${promptName}`,
		promptParams || {}
	)) as McpServerGeneratedPrompt;
	return response;
}

export async function listProjectMcpServerResources(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string
) {
	const response = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/resources`,
		{
			dontLogErrors: true
		}
	)) as McpServerResource[];
	return response;
}

export async function readProjectMcpServerResource(
	assistantID: string,
	projectID: string,
	projectMcpServerId: string,
	resourceUri: string
) {
	const encodedResourceUri = encodeURIComponent(resourceUri);
	const response = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/mcpservers/${projectMcpServerId}/resources/${encodedResourceUri}`
	)) as McpServerResourceContent[];
	return response[0];
}

export async function listProjectMembers(
	assistantID: string,
	projectID: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectMember[]> {
	const response = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/members`,
		opts
	)) as ProjectMember[];
	return response.sort((a, b) => {
		if (a.isOwner && !b.isOwner) return -1;
		if (!a.isOwner && b.isOwner) return 1;
		return 0;
	});
}

export async function deleteProjectMember(
	assistantID: string,
	projectID: string,
	memberID: string
) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/members/${memberID}`);
}

export async function createProjectInvitation(
	assistantID: string,
	projectID: string
): Promise<ProjectInvitation> {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/invitations`,
		{}
	)) as ProjectInvitation;
}

export async function listProjectInvitations(
	assistantID: string,
	projectID: string
): Promise<ProjectInvitation[]> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/invitations`
	)) as ProjectInvitation[];
}

export async function deleteProjectInvitation(
	assistantID: string,
	projectID: string,
	code: string
) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/invitations/${code}`);
}

export async function getProjectInvitation(
	code: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectInvitation> {
	return (await doGet(`/projectinvitations/${code}`, opts)) as ProjectInvitation;
}

export async function acceptProjectInvitation(code: string): Promise<ProjectInvitation> {
	return (await doPost(
		`/projectinvitations/${code}`,
		{},
		{ dontLogErrors: true }
	)) as ProjectInvitation;
}

export async function rejectProjectInvitation(code: string): Promise<void> {
	return doDelete(`/projectinvitations/${code}`) as unknown as Promise<void>;
}

export async function createProjectTemplate(
	assistantID: string,
	projectID: string
): Promise<ProjectTemplate> {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/templates`,
		{}
	)) as ProjectTemplate;
}

export async function getProjectTemplate(
	assistantID: string,
	projectID: string,
	templateID: string
): Promise<ProjectTemplate> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/templates/${templateID}`
	)) as ProjectTemplate;
}

export async function listProjectTemplates(
	assistantID: string,
	projectID: string
): Promise<ProjectTemplateList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/templates`
	)) as ProjectTemplateList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function updateProjectTemplate(
	assistantID: string,
	projectID: string,
	templateID: string,
	template: ProjectTemplateManifest
): Promise<ProjectTemplate> {
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/templates/${templateID}`,
		template
	)) as ProjectTemplate;
}

export async function deleteProjectTemplate(
	assistantID: string,
	projectID: string,
	templateID: string
) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/templates/${templateID}`);
}

export async function listTemplates(opts?: {
	all?: boolean;
	fetch?: Fetcher;
}): Promise<ProjectTemplateList> {
	const queryParams = opts?.all ? '?all=true' : '';
	const list = (await doGet(`/templates${queryParams}`, opts)) as ProjectTemplateList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function getTemplate(
	publicID: string,
	opts?: { fetch?: Fetcher }
): Promise<ProjectTemplate> {
	return (await doGet(`/templates/${publicID}`, opts)) as ProjectTemplate;
}

export async function copyTemplate(publicID: string, opts?: { fetch?: Fetcher }): Promise<Project> {
	return (await doPost(`/templates/${publicID}`, {}, opts)) as Project;
}

export async function listAvailableModels(
	assistantID: string,
	projectID: string,
	providerId: string
): Promise<ChatModelList> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/model-providers/${providerId}/available-models`
	)) as ChatModelList;
}

// Model provider operations
export async function listModelProviders(
	assistantID: string,
	projectID: string
): Promise<ModelProviderList> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/model-providers`
	)) as ModelProviderList;
}

export async function configureModelProvider(
	assistantID: string,
	projectID: string,
	providerId: string,
	config: Record<string, string>
): Promise<void> {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/model-providers/${providerId}/configure`,
		config
	) as Promise<void>;
}

export async function deconfigureModelProvider(
	assistantID: string,
	projectID: string,
	providerId: string
): Promise<void> {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/model-providers/${providerId}/deconfigure`,
		{}
	) as Promise<void>;
}

export async function getModelProviderConfig(
	assistantID: string,
	projectID: string,
	providerId: string
): Promise<Record<string, string>> {
	return doPost(
		`/assistants/${assistantID}/projects/${projectID}/model-providers/${providerId}/reveal`,
		{},
		{ dontLogErrors: true }
	) as Promise<Record<string, string>>;
}

export async function getDefaultModelForThread(
	assistantID: string,
	projectID: string,
	threadID: string
): Promise<{ model: string; modelProvider: string }> {
	try {
		return (await doGet(
			`/assistants/${assistantID}/projects/${projectID}/threads/${threadID}/default-model`,
			{ dontLogErrors: true }
		)) as { model: string; modelProvider: string };
	} catch {
		return { model: '', modelProvider: '' };
	}
}

export async function getBootstrapStatus(): Promise<{ enabled: boolean }> {
	return (await doGet('/bootstrap')) as { enabled: boolean };
}

export async function bootstrapLogin(token: string) {
	return doPost('/bootstrap/login', {
		headers: {
			Authorization: `Bearer ${token}}`
		}
	});
}

export async function bootstrapLogout() {
	return doPost('/bootstrap/logout', {});
}

export async function listSingleOrRemoteMcpServers(opts?: {
	fetch?: Fetcher;
}): Promise<MCPServer[]> {
	const response = (await doGet('/mcp-servers', opts)) as ItemsResponse<MCPServer>;
	return response.items ?? [];
}

export async function getSingleOrRemoteMcpServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPServer> {
	const response = (await doGet(`/mcp-servers/${id}`, opts)) as MCPServer;
	return response;
}

export async function createSingleOrRemoteMcpServer(server: MCPServer): Promise<MCPServer> {
	const response = (await doPost('/mcp-servers', server)) as MCPServer;
	return response;
}

export async function updateSingleOrRemoteMcpServer(
	id: string,
	server: MCPServer
): Promise<MCPServer> {
	const response = (await doPut(`/mcp-servers/${id}`, server)) as MCPServer;
	return response;
}

export async function deleteSingleOrRemoteMcpServer(id: string): Promise<void> {
	await doDelete(`/mcp-servers/${id}`);
}

export async function configureSingleOrRemoteMcpServer(
	id: string,
	envs: Record<string, string>
): Promise<void> {
	await doPost(`/mcp-servers/${id}/configure`, envs);
}

export async function deconfigureSingleOrRemoteMcpServer(id: string): Promise<void> {
	await doPost(`/mcp-servers/${id}/deconfigure`, {});
}

export async function revealSingleOrRemoteMcpServer(id: string): Promise<Record<string, string>> {
	return doPost(`/mcp-servers/${id}/reveal`, {}) as Promise<Record<string, string>>;
}

export async function listSingleOrRemoteMcpServerTools(id: string): Promise<MCPServerTool[]> {
	const response = (await doGet(`/mcp-servers/${id}/tools`)) as ItemsResponse<MCPServerTool>;
	return response.items ?? [];
}

export async function listSingleOrRemoteMcpServerPrompts(id: string): Promise<MCPServerPrompt[]> {
	const response = (await doGet(`/mcp-servers/${id}/prompts`)) as ItemsResponse<MCPServerPrompt>;
	return response.items ?? [];
}

export async function listSingleOrRemoteMcpServerResources(
	id: string
): Promise<McpServerResource[]> {
	const response = (await doGet(
		`/mcp-servers/${id}/resources`
	)) as ItemsResponse<McpServerResource>;
	return response.items ?? [];
}

export async function listMcpServerInstances(opts?: {
	fetch?: Fetcher;
}): Promise<MCPServerInstance[]> {
	const response = (await doGet('/mcp-server-instances', opts)) as ItemsResponse<MCPServerInstance>;
	return response.items ?? [];
}

export async function getMcpServerInstance(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerInstance> {
	const response = (await doGet(`/mcp-server-instances/${id}`, opts)) as MCPServerInstance;
	return response;
}

export async function createMcpServerInstance(
	instance: MCPServerInstance
): Promise<MCPServerInstance> {
	const response = (await doPost('/mcp-server-instances', instance)) as MCPServerInstance;
	return response;
}

export async function deleteMcpServerInstance(id: string): Promise<void> {
	await doDelete(`/mcp-server-instances/${id}`);
}
