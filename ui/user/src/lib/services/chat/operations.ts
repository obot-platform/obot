import { baseURL, doDelete, doGet, doPost, doPut } from './http';
import {
	type AuthProvider,
	type AuthProviderList,
	type AssistantToolList,
	type AssistantTool,
	type Assistant,
	type Assistants,
	type Files,
	type InvokeInput,
	type KnowledgeFile,
	type KnowledgeFiles,
	type Profile,
	type Task,
	type TaskList,
	type TaskRun,
	type TaskRunList,
	type Thread,
	type ThreadList,
	type Version,
	type TableList,
	type Rows,
	type ProjectList,
	type Project,
	type ProjectShareList,
	type ProjectAuthorizationList,
	type ProjectCredentialList,
	type ProjectShare,
	type ToolReferenceList
} from './types';

export type Fetcher = typeof fetch;

export async function getProfile(): Promise<Profile> {
	const obj = (await doGet('/me')) as Profile;
	obj.isAdmin = () => {
		return obj.role === 1;
	};
	obj.getDisplayName = () => {
		return obj?.currentAuthProvider === 'github-auth-provider' ? obj.username : obj?.email;
	};
	obj.loaded = true;
	return obj;
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

export async function createTool(
	assistantID: string,
	projectID: string,
	tool?: AssistantTool,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<AssistantTool> {
	const result = (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/tools`,
		tool ?? {}
	)) as AssistantTool;
	if (opts?.env) {
		await saveToolEnv(assistantID, projectID, result.id, opts.env);
	}
	return result;
}

export async function listAllTools(opts?: { fetch: Fetcher }): Promise<ToolReferenceList> {
	const list = (await doGet(`/tool-references?type=tool`, opts)) as ToolReferenceList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function testTool(
	assistantID: string,
	projectID: string,
	tool: AssistantTool,
	input: object,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<{ output: string }> {
	return (await doPost(
		`/assistants/${assistantID}/projects/${projectID}/tools/${tool.id}/test`,
		{
			input,
			tool,
			env: opts?.env
		},
		{
			dontLogErrors: true
		}
	)) as {
		output: string;
	};
}

export async function updateTool(
	assistantID: string,
	projectID: string,
	tool: AssistantTool,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<AssistantToolList> {
	const result = (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/tools/${tool.id}`,
		tool
	)) as AssistantToolList;
	if (opts?.env) {
		await saveToolEnv(assistantID, projectID, tool.id, opts.env);
	}
	return result;
}

export async function deleteTool(assistantID: string, projectID: string, tool: string) {
	return doDelete(`/assistants/${assistantID}/projects/${projectID}/tools/${tool}/custom`);
}

export async function getTool(
	assistantID: string,
	projectID: string,
	tool: string
): Promise<AssistantTool> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tools/${tool}`
	)) as AssistantTool;
}

export async function getToolEnv(
	assistantID: string,
	projectID: string,
	tool: string
): Promise<Record<string, string>> {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tools/${tool}/env`
	)) as Record<string, string>;
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

export async function saveToolEnv(
	assistantID: string,
	projectID: string,
	tool: string,
	env: Record<string, string>
): Promise<Record<string, string>> {
	return (await doPut(
		`/assistants/${assistantID}/projects/${projectID}/tools/${tool}/env`,
		env
	)) as Record<string, string>;
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
		input?: string | object;
	}
): Promise<TaskRun> {
	const url = `/assistants/${assistantID}/projects/${projectID}/tasks/${taskID}/run?step=${opts?.stepID ?? ''}`;
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

export async function createThread(assistantID: string, projectID: string): Promise<Thread> {
	return (await doPost(`/assistants/${assistantID}/projects/${projectID}/threads`, {})) as Thread;
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
	return list;
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

export async function deleteProject(assistantID: string, id: string) {
	return doDelete(`/assistants/${assistantID}/projects/${id}`);
}

export async function updateProject(project: Project): Promise<Project> {
	return (await doPut(
		`/assistants/${project.assistantID}/projects/${project.id}`,
		project
	)) as Project;
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
	}
): EventSource {
	if (opts?.authenticate?.tools) {
		let url = `/assistants/${assistantID}/projects/${projectID}/tools/${opts.authenticate.tools.join(',')}/authenticate`;
		if (opts.authenticate.local) {
			url = `/assistants/${assistantID}/projects/${projectID}/tools/${opts.authenticate.tools.join(',')}/local-authenticate`;
		}
		return new EventSource(baseURL + url);
	}
	if (opts?.task) {
		let url = `/assistants/${assistantID}/projects/${projectID}/tasks/${opts.task.id}/events`;
		if (opts.runID) {
			url = `/assistants/${assistantID}/projects/${projectID}/tasks/${opts.task.id}/runs/${opts.runID}/events`;
		}
		return new EventSource(baseURL + `${url}`);
	}
	return new EventSource(
		baseURL + `/assistants/${assistantID}/projects/${projectID}/threads/${opts?.threadID}/events`
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

export async function listTaskRuns(
	assistantID: string,
	projectID: string,
	id: string
): Promise<TaskRunList> {
	const list = (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tasks/${id}/runs`
	)) as TaskRunList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listTables(assistantID: string, projectID: string): Promise<TableList> {
	return (await doGet(`/assistants/${assistantID}/projects/${projectID}/tables`)) as TableList;
}

export async function getRows(assistantID: string, projectID: string, table: string) {
	return (await doGet(
		`/assistants/${assistantID}/projects/${projectID}/tables/${table}/rows`
	)) as Rows;
}

export async function sendCredentials(id: string, credentials: Record<string, string>) {
	return await doPost('/prompt', { id, response: credentials });
}

export async function listAuthProviders(opts?: { fetch?: Fetcher }): Promise<AuthProvider[]> {
	const list = (await doGet('/auth-providers', opts)) as AuthProviderList;
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
	opts?: { fetch?: Fetcher }
): Promise<Project> {
	return (await doPost(`/shares/${id}`, {}, opts)) as Project;
}

export async function listProjectShares(opts?: { fetch?: Fetcher }): Promise<ProjectShareList> {
	const list = (await doGet(`/shares`, opts)) as ProjectShareList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function copyProject(assistantID: string, projectID: string): Promise<Project> {
	return (await doPost(`/assistants/${assistantID}/projects/${projectID}/copy`, {})) as Project;
}
