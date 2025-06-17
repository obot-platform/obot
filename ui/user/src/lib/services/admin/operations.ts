import type { ModelProvider, Project, Task } from '../chat/types';
import { doDelete, doGet, doPatch, doPost, doPut, type Fetcher } from '../http';
import type {
	AuthProvider,
	FileScannerConfig,
	FileScannerProvider,
	MCPCatalog,
	MCPCatalogEntry,
	MCPCatalogEntryManifest,
	MCPCatalogManifest,
	OrgUser,
	Model,
	ProjectThread
} from './types';

type ItemsResponse<T> = { items: T[] | null };

export async function listMCPCatalogs(opts?: { fetch?: Fetcher }): Promise<MCPCatalog[]> {
	const response = (await doGet('/mcp-catalogs', opts)) as ItemsResponse<MCPCatalog>;
	return response.items ?? [];
}

export async function getMCPCatalog(id: string, opts?: { fetch?: Fetcher }): Promise<MCPCatalog> {
	const response = (await doGet(`/mcp-catalogs/${id}`, opts)) as MCPCatalog;
	return response;
}

export async function refreshMCPCatalog(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalog> {
	const response = (await doPost(`/mcp-catalogs/${id}/refresh`, {}, opts)) as MCPCatalog;
	return response;
}

export async function createMCPCatalog(
	catalog: MCPCatalogManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalog> {
	const response = (await doPost(`/mcp-catalogs`, catalog, opts)) as MCPCatalog;
	return response;
}

export async function updateMCPCatalog(
	id: string,
	catalog: MCPCatalogManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalog> {
	const response = (await doPut(`/mcp-catalogs/${id}`, catalog, opts)) as MCPCatalog;
	return response;
}

export async function deleteMCPCatalog(id: string): Promise<void> {
	await doDelete(`/mcp-catalogs/${id}`);
}

export async function listMCPCatalogEntries(
	catalogID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry[]> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/entries`,
		opts
	)) as ItemsResponse<MCPCatalogEntry>;
	return response.items ?? [];
}

export async function createMCPCatalogEntry(
	catalogID: string,
	entry: MCPCatalogEntryManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/entries`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return response;
}

export async function updateMCPCatalogEntry(
	catalogID: string,
	entryID: string,
	entry: MCPCatalogEntryManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPut(
		`/mcp-catalogs/${catalogID}/entries/${entryID}`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return response;
}

export async function deleteMCPCatalogEntry(catalogID: string, entryID: string): Promise<void> {
	await doDelete(`/mcp-catalogs/${catalogID}/entries/${entryID}`);
}

export async function listUsers(opts?: { fetch?: Fetcher }): Promise<OrgUser[]> {
	const response = (await doGet('/users', opts)) as ItemsResponse<OrgUser>;
	return response.items ?? [];
}

export async function updateUserRole(
	userID: string,
	role: number,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPatch(`/users/${userID}`, { role }, opts);
}

export async function deleteUser(userID: string): Promise<void> {
	await doDelete(`/users/${userID}`);
}

export async function listThreads(opts?: { fetch?: Fetcher }): Promise<ProjectThread[]> {
	const response = (await doGet('/threads', opts)) as ItemsResponse<ProjectThread>;
	return response.items ?? [];
}

export async function listProjects(opts?: { fetch?: Fetcher }): Promise<Project[]> {
	const response = (await doGet('/projects?all=true', opts)) as ItemsResponse<Project>;
	return response.items ?? [];
}

export async function listTasks(opts?: { fetch?: Fetcher }): Promise<Task[]> {
	const response = (await doGet('/tasks', opts)) as ItemsResponse<Task>;
	return response.items ?? [];
}

export async function listModelProviders(opts?: { fetch?: Fetcher }): Promise<ModelProvider[]> {
	const response = (await doGet('/model-providers', opts)) as ItemsResponse<ModelProvider>;
	return response.items ?? [];
}

export async function listModels(opts?: { fetch?: Fetcher }): Promise<Model[]> {
	const response = (await doGet('/models', opts)) as ItemsResponse<Model>;
	return response.items ?? [];
}

export async function listAuthProviders(opts?: { fetch?: Fetcher }): Promise<AuthProvider[]> {
	const response = (await doGet('/auth-providers', opts)) as ItemsResponse<AuthProvider>;
	return response.items ?? [];
}

export async function listFileScannerProviders(opts?: {
	fetch?: Fetcher;
}): Promise<FileScannerProvider[]> {
	const response = (await doGet(
		'/file-scanner-providers',
		opts
	)) as ItemsResponse<FileScannerProvider>;
	return response.items ?? [];
}

export async function getFileScannerConfig(opts?: { fetch?: Fetcher }): Promise<FileScannerConfig> {
	const response = (await doGet('/file-scanner-config', opts)) as FileScannerConfig;
	return response;
}

export async function deleteProject(assistantID: string, projectID: string): Promise<void> {
	await doDelete(`/assistants/${assistantID}/projects/${projectID}`);
}
