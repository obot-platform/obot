import context from '$lib/stores/context.svelte';
import { baseURL, doDelete, doGet, doPost, doPut } from './http';
import {
	type AuthProvider,
	type AuthProviderList,
	type AssistantToolList,
	type AssistantTool,
	type Assistant,
	type Assistants,
	type CredentialList,
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
	type ProjectAuthorizationList,
	type ProjectCredentialList,
	type ProjectTemplate,
	type ProjectTemplateList
} from './types';

function assistantID(): string {
	return context.assistantID;
}

function projectID(): string {
	return context.projectID;
}

function currentThreadID(): string | undefined {
	return context.currentThreadID;
}

export async function getProfile(): Promise<Profile> {
	const obj = (await doGet('/me')) as Profile;
	obj.isAdmin = () => {
		return obj.role === 1;
	};
	obj.loaded = true;
	return obj;
}

export async function getVersion(): Promise<Version> {
	return (await doGet('/version')) as Version;
}

export async function getAssistant(id: string): Promise<Assistant> {
	return (await doGet(`/assistants/${id}`)) as Assistant;
}

export async function listAssistants(): Promise<Assistants> {
	const assistants = (await doGet(`/assistants`)) as Assistants;
	if (!assistants.items) {
		assistants.items = [];
	}
	return assistants;
}

export async function deleteKnowledgeFile(filename: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/knowledge/${filename}`);
}

export async function deleteFile(filename: string, opts?: { taskID?: string; runID?: string }) {
	if (opts?.taskID && opts?.runID) {
		return doDelete(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/files/${filename}`
		);
	}
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/files/${filename}`);
}

export async function download(
	filename: string,
	opts?: {
		taskID?: string;
		runID?: string;
	}
) {
	let url = `/assistants/${assistantID()}/projects/${projectID()}/file/${filename}`;
	if (opts?.taskID && opts?.runID) {
		url = `/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`;
	}
	url = baseURL + url;

	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
}

export async function saveFile(file: File, opts?: { taskID?: string; runID?: string }) {
	if (opts?.taskID && opts?.runID) {
		return (await doPost(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/file/${file.name}`,
			file
		)) as Files;
	}
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/file/${file.name}`,
		file
	)) as Files;
}

export async function saveContents(
	filename: string,
	contents: string,
	opts?: { taskID?: string; runID?: string }
) {
	if (opts?.taskID && opts?.runID) {
		return (await doPost(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`,
			contents
		)) as Files;
	}
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/file/${filename}`,
		contents
	)) as Files;
}

export async function getFile(
	filename: string,
	opts?: {
		taskID?: string;
		runID?: string;
	}
): Promise<Blob> {
	if (opts?.taskID && opts?.runID) {
		return (await doGet(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/file/${filename}`,
			{
				blob: true
			}
		)) as Blob;
	}
	return (await doGet(`/assistants/${assistantID()}/projects/${projectID()}/file/${filename}`, {
		blob: true
	})) as Blob;
}

export async function uploadKnowledge(file: File): Promise<KnowledgeFile> {
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/knowledge/${file.name}`,
		file
	)) as KnowledgeFile;
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

export async function listKnowledgeFiles(opts?: { projectID?: string }): Promise<KnowledgeFiles> {
	const p = opts?.projectID ?? projectID();
	const files = (await doGet(
		`/assistants/${assistantID()}/projects/${p}/knowledge`
	)) as KnowledgeFiles;
	if (!files.items) {
		files.items = [];
	}
	return removedDeleted(files);
}

export async function listFiles(opts?: { taskID?: string; runID?: string }): Promise<Files> {
	let files: Files;
	if (opts?.taskID && opts?.runID) {
		files = (await doGet(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/files`
		)) as Files;
	} else {
		files = (await doGet(`/assistants/${assistantID()}/projects/${projectID()}/files`)) as Files;
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

export async function invoke(msg: string | InvokeInput) {
	msg = cleanInvokeInput(msg);
	await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/threads/${currentThreadID()}/invoke`,
		msg
	);
}

export async function abort(opts?: { threadID?: string; taskID?: string; runID?: string }) {
	if (opts?.taskID && opts?.runID) {
		return await doPost(
			`/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.taskID}/runs/${opts.runID}/abort`,
			{}
		);
	}
	await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/threads/${opts?.threadID}/abort`,
		{}
	);
}

export async function listCredentials(): Promise<CredentialList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/credentials`
	)) as CredentialList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function deleteCredential(id: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/credentials/${id}`);
}

export async function listTools(): Promise<AssistantToolList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tools`
	)) as AssistantToolList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function createTool(
	tool?: AssistantTool,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<AssistantTool> {
	const result = (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/tools`,
		tool ?? {}
	)) as AssistantTool;
	if (opts?.env) {
		await saveToolEnv(result.id, opts.env);
	}
	return result;
}

export async function testTool(
	tool: AssistantTool,
	input: object,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<{ output: string }> {
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool.id}/test`,
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
	tool: AssistantTool,
	opts?: {
		env?: Record<string, string>;
	}
): Promise<AssistantToolList> {
	const result = (await doPut(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool.id}`,
		tool
	)) as AssistantToolList;
	if (opts?.env) {
		await saveToolEnv(tool.id, opts.env);
	}
	return result;
}

export async function deleteTool(tool: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}/custom`);
}

export async function getTool(tool: string): Promise<AssistantTool> {
	return (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}`
	)) as AssistantTool;
}

export async function getToolEnv(tool: string): Promise<Record<string, string>> {
	return (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}/env`
	)) as Record<string, string>;
}

export async function getAssistantEnv(): Promise<Record<string, string>> {
	return (await doGet(`/assistants/${assistantID()}/projects/${projectID()}/env`)) as Record<
		string,
		string
	>;
}

export async function saveAssistantEnv(
	env: Record<string, string>
): Promise<Record<string, string>> {
	return (await doPut(`/assistants/${assistantID()}/projects/${projectID()}/env`, env)) as Record<
		string,
		string
	>;
}

export async function saveToolEnv(
	tool: string,
	env: Record<string, string>
): Promise<Record<string, string>> {
	return (await doPut(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}/env`,
		env
	)) as Record<string, string>;
}

export async function enableTool(tool: string): Promise<AssistantToolList> {
	return (await doPut(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}`
	)) as AssistantToolList;
}

export async function disableTool(tool: string): Promise<AssistantToolList> {
	return (await doDelete(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${tool}`
	)) as AssistantToolList;
}

export async function saveTask(task: Task): Promise<Task> {
	return (await doPut(
		`/assistants/${assistantID()}/projects/${projectID()}/tasks/${task.id}`,
		task
	)) as Task;
}

export async function runTask(
	taskID: string,
	opts?: {
		stepID?: string;
		input?: string | object;
	}
): Promise<TaskRun> {
	const url = `/assistants/${assistantID()}/projects/${projectID()}/tasks/${taskID}/run?step=${opts?.stepID ?? ''}`;
	return (await doPost(url, opts?.input ?? {})) as TaskRun;
}

export async function deleteThread(threadID: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/threads/${threadID}`);
}

export async function updateThread(thread: Thread): Promise<Thread> {
	return (await doPut(
		`/assistants/${assistantID()}/projects/${projectID()}/threads/${thread.id}`,
		thread
	)) as Thread;
}

export async function deleteProjectTemplate(projectID: string, projectTemplateID: string) {
	return doDelete(
		`/assistants/${assistantID()}/projects/${projectID}/templates/${projectTemplateID}`
	);
}

export async function listProjectTemplates(opts?: {
	projectID?: string;
}): Promise<ProjectTemplateList> {
	let url = '/templates';
	if (opts?.projectID) {
		url = `/assistants/${assistantID()}/projects/${opts.projectID}/templates`;
	}
	const list = (await doGet(url)) as ProjectTemplateList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function getPublicTemplate(templateID: string): Promise<ProjectTemplate | undefined> {
	try {
		return (await doGet(`/templates/${templateID}`, {
			dontLogErrors: true
		})) as ProjectTemplate;
	} catch (e) {
		if ((e as Error).message.includes('404')) {
			return;
		}
		throw e;
	}
}

export async function getProjectTemplate(
	projectID: string,
	projectTemplateID: string
): Promise<ProjectTemplate> {
	return (await doGet(
		`/assistants/${assistantID()}/projects/${projectID}/templates/${projectTemplateID}`
	)) as ProjectTemplate;
}

export async function createProjectTemplate(
	projectID: string,
	opts?: {
		shareCredentials?: boolean;
	}
): Promise<ProjectTemplate> {
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID}/templates`,
		opts ?? {}
	)) as ProjectTemplate;
}

export async function createThread(): Promise<Thread> {
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/threads`,
		{}
	)) as Thread;
}

export async function listThreads(): Promise<ThreadList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/threads`
	)) as ThreadList;
	if (!list.items) {
		list.items = [];
	}
	list.items.sort((a, b) => {
		return b.created.localeCompare(a.created);
	});
	return list;
}

export async function acceptPendingAuthorization(projectID: string) {
	return doPut(`/assistants/${assistantID()}/pending-authorizations/${projectID}`, {});
}

export async function rejectPendingAuthorization(projectID: string) {
	return doDelete(`/assistants/${assistantID()}/pending-authorizations/${projectID}`);
}

export async function listPendingAuthorizations(): Promise<ProjectAuthorizationList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/pending-authorizations`
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function updateProjectTools(
	projectID: string,
	tools: AssistantToolList
): Promise<AssistantToolList> {
	const list = (await doPut(
		`/assistants/${assistantID()}/projects/${projectID}/tools`,
		tools
	)) as AssistantToolList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function updateProjectAuthorizations(
	projectID: string,
	authorizations: ProjectAuthorizationList
): Promise<ProjectAuthorizationList> {
	const list = (await doPut(
		`/assistants/${assistantID()}/projects/${projectID}/authorizations`,
		authorizations
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listProjectCredentials(projectID: string): Promise<ProjectCredentialList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID}/credentials`
	)) as ProjectCredentialList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listProjectAuthorizations(id: string): Promise<ProjectAuthorizationList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${id}/authorizations`
	)) as ProjectAuthorizationList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function deleteProjectCredential(toolID: string) {
	return doDelete(
		`/assistants/${assistantID()}/projects/${projectID()}/tools/${toolID}/deauthenticate`
	);
}

export async function createProjectFromTemplate(
	template: ProjectTemplate,
	opts?: {
		name?: string;
	}
): Promise<Project> {
	return (await doPost(`/templates/${template.id}/projects`, opts ?? {})) as Project;
}

export async function createProject(opts?: { name: string; default?: boolean }): Promise<Project> {
	return (await doPost(`/assistants/${assistantID()}/projects`, opts ?? {})) as Project;
}

export async function getProject(id: string): Promise<Project> {
	return (await doGet(`/assistants/${assistantID()}/projects/${id}`)) as Project;
}

export async function deleteProject(id: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${id}`);
}

export async function updateProject(project: Project): Promise<Project> {
	return (await doPut(`/assistants/${assistantID()}/projects/${project.id}`, project)) as Project;
}

export async function listProjects(opts?: { assistantID?: string }): Promise<ProjectList> {
	let url = '/projects';
	if (opts?.assistantID) {
		url = `/assistants/${opts.assistantID}/projects`;
	}
	const list = (await doGet(url)) as ProjectList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export function newMessageEventSource(opts?: {
	authenticate?: {
		tools?: string[];
	};
	threadID?: string;
	task?: {
		id: string;
	};
	runID?: string;
}): EventSource {
	if (opts?.authenticate?.tools) {
		const url = `/assistants/${assistantID()}/projects/${projectID()}/tools/${opts.authenticate.tools.join(',')}/authenticate`;
		return new EventSource(baseURL + url);
	}
	if (opts?.task) {
		let url = `/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.task.id}/events`;
		if (opts.runID) {
			url = `/assistants/${assistantID()}/projects/${projectID()}/tasks/${opts.task.id}/runs/${opts.runID}/events`;
		}
		return new EventSource(baseURL + `${url}`);
	}
	return new EventSource(
		baseURL +
			`/assistants/${assistantID()}/projects/${projectID()}/threads/${opts?.threadID}/events`
	);
}

export async function listTasks(): Promise<TaskList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tasks`
	)) as TaskList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function createTask(task?: Task): Promise<Task> {
	return (await doPost(
		`/assistants/${assistantID()}/projects/${projectID()}/tasks`,
		task ?? {
			steps: []
		}
	)) as Task;
}

export async function deleteTask(id: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/tasks/${id}`);
}

export async function getTask(id: string): Promise<Task> {
	return (await doGet(`/assistants/${assistantID()}/projects/${projectID()}/tasks/${id}`)) as Task;
}

export async function getTaskRun(taskID: string, runID: string): Promise<TaskRun> {
	return (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tasks/${taskID}/runs/${runID}`
	)) as TaskRun;
}

export async function deleteTaskRun(id: string, runID: string) {
	return doDelete(`/assistants/${assistantID()}/projects/${projectID()}/tasks/${id}/runs/${runID}`);
}

export async function listTaskRuns(id: string): Promise<TaskRunList> {
	const list = (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tasks/${id}/runs`
	)) as TaskRunList;
	if (!list.items) {
		list.items = [];
	}
	return list;
}

export async function listTables() {
	return (await doGet(`/assistants/${assistantID()}/projects/${projectID()}/tables`)) as TableList;
}

export async function getRows(table: string) {
	return (await doGet(
		`/assistants/${assistantID()}/projects/${projectID()}/tables/${table}/rows`
	)) as Rows;
}

export async function sendCredentials(id: string, credentials: Record<string, string>) {
	return await doPost('/prompt', { id, response: credentials });
}

export async function listAuthProviders(): Promise<AuthProvider[]> {
	const list = (await doGet('/auth-providers')) as AuthProviderList;
	if (!list.items) {
		list.items = [];
	}
	return list.items.filter((provider) => provider.configured);
}
