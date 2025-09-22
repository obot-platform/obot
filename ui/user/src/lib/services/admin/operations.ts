import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import type {
	ModelProvider,
	Project,
	MCPCatalogServer,
	MCPServerInstance,
	Model
} from '../chat/types';
import { doDelete, doGet, doPatch, doPost, doPut, type Fetcher } from '../http';
import type {
	FileScannerConfig,
	FileScannerProvider,
	MCPCatalog,
	MCPCatalogEntry,
	MCPCatalogEntryServerManifest,
	MCPCatalogManifest,
	OrgUser,
	OrgGroup,
	ProjectThread,
	MCPCatalogServerManifest,
	DefaultModelAlias,
	ModelAlias,
	AccessControlRule,
	AccessControlRuleManifest,
	AuthProvider,
	BootstrapStatus,
	AuditLog,
	AuditLogUsageStats,
	AuditLogURLFilters,
	K8sServerDetail,
	BaseAgent,
	MCPFilter,
	MCPFilterManifest,
	ProjectTask
} from './types';

type ItemsResponse<T> = { items: T[] | null };
export type PaginatedResponse<T> = {
	items: T[] | null;
	total: number;
	offset: number;
	limit: number;
};

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

export async function updateMCPCatalog(
	id: string,
	catalog: MCPCatalogManifest,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<MCPCatalog> {
	const response = (await doPut(`/mcp-catalogs/${id}`, catalog, opts)) as MCPCatalog;
	return response;
}

export async function listMCPCatalogEntries(
	catalogID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry[]> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/entries`,
		opts
	)) as ItemsResponse<MCPCatalogEntry>;
	return (
		response.items?.map((item) => {
			return {
				...item,
				isCatalogEntry: true
			};
		}) ?? []
	);
}

export async function getMCPCatalogEntry(
	catalogID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/entries/${entryID}`,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function createMCPCatalogEntry(
	catalogID: string,
	entry: MCPCatalogEntryServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/entries`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function updateMCPCatalogEntry(
	catalogID: string,
	entryID: string,
	entry: MCPCatalogEntryServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPut(
		`/mcp-catalogs/${catalogID}/entries/${entryID}`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function deleteMCPCatalogEntry(catalogID: string, entryID: string): Promise<void> {
	await doDelete(`/mcp-catalogs/${catalogID}/entries/${entryID}`);
}

export async function listMCPServersForEntry(
	catalogID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer[]> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/entries/${entryID}/servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function createMCPCatalogServer(
	catalogID: string,
	server: MCPCatalogServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/servers`,
		server,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function listMcpCatalogServerInstances(
	catalogId: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = (await doGet(
		`/mcp-catalogs/${catalogId}/servers/${mcpServerId}/instances`,
		opts
	)) as ItemsResponse<MCPServerInstance>;
	return response.items ?? [];
}

export async function updateMCPCatalogServer(
	catalogID: string,
	serverID: string,
	server: MCPCatalogServerManifest['manifest'],
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPut(
		`/mcp-catalogs/${catalogID}/servers/${serverID}`,
		server,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function deleteMCPCatalogServer(catalogID: string, serverID: string): Promise<void> {
	await doDelete(`/mcp-catalogs/${catalogID}/servers/${serverID}`);
}

export async function listMCPCatalogServers(
	catalogID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer[]> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function configureMCPCatalogServer(
	catalogID: string,
	serverID: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/servers/${serverID}/configure`,
		envs,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function revealMcpCatalogServer(
	catalogID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<Record<string, string>> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/servers/${serverID}/reveal`,
		{},
		{
			...opts,
			dontLogErrors: true
		}
	)) as Record<string, string>;
	return response;
}

export async function getMCPCatalogServer(
	catalogID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/servers/${serverID}`,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function getMCPCatalogServerOAuthURL(
	catalogID: string,
	serverID: string,
	opts?: { signal?: AbortSignal }
): Promise<string> {
	try {
		const response = (await doGet(`/mcp-catalogs/${catalogID}/servers/${serverID}/oauth-url`, {
			dontLogErrors: true,
			signal: opts?.signal
		})) as {
			oauthURL: string;
		};
		return response.oauthURL;
	} catch (_err) {
		return '';
	}
}

export async function isMCPCatalogServerOauthNeeded(
	catalogID: string,
	serverID: string,
	opts?: { signal?: AbortSignal }
): Promise<boolean> {
	try {
		await doPost(`/mcp-catalogs/${catalogID}/servers/${serverID}/check-oauth`, {
			dontLogErrors: true,
			signal: opts?.signal
		});
	} catch (err) {
		if (err instanceof Error && err.message.includes('412')) {
			return true;
		}
	}
	return false;
}

export async function deconfigureMCPCatalogServer(
	catalogID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/mcp-catalogs/${catalogID}/servers/${serverID}/deconfigure`, {}, opts);
}

export async function generateMcpCatalogEntryToolPreviews(
	catalogID: string,
	entryID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
	},
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/mcp-catalogs/${catalogID}/entries/${entryID}/generate-tool-previews`, body ?? {}, {
		...opts,
		dontLogErrors: true
	});
}

export async function getMcpCatalogToolPreviewsOauth(
	catalogID: string,
	entryID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
	},
	opts?: { fetch?: Fetcher }
): Promise<string> {
	try {
		const response = (await doPost(
			`/mcp-catalogs/${catalogID}/entries/${entryID}/generate-tool-previews/oauth-url`,
			body ?? {},
			{
				...opts,
				dontLogErrors: true
			}
		)) as {
			oauthURL: string;
		};
		return response.oauthURL;
	} catch (_err) {
		return '';
	}
}

export async function listUsers(opts?: { fetch?: Fetcher }): Promise<OrgUser[]> {
	const response = (await doGet('/users', opts)) as ItemsResponse<OrgUser>;
	return response.items ?? [];
}

export async function listUsersIncludeDeleted(opts?: { fetch?: Fetcher }): Promise<OrgUser[]> {
	const response = (await doGet('/users?includeDeleted=true', opts)) as ItemsResponse<OrgUser>;
	return response.items ?? [];
}

export async function getUser(
	userID: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<OrgUser> {
	const response = (await doGet(`/users/${userID}`, opts)) as OrgUser;
	return response;
}

export async function listGroups(opts?: { fetch?: Fetcher; query?: string }): Promise<OrgGroup[]> {
	const queryParam = opts?.query ? `?name=${encodeURIComponent(opts.query)}` : '';
	const response = (await doGet(`/groups${queryParam}`, opts)) as OrgGroup[];
	return response ?? [];
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

export async function listProjects(opts?: { fetch?: Fetcher }): Promise<Project[]> {
	const response = (await doGet('/projects?all=true', opts)) as ItemsResponse<Project>;
	return response.items ?? [];
}

export async function listThreads(opts?: { fetch?: Fetcher }): Promise<ProjectThread[]> {
	const response = (await doGet('/threads', opts)) as ItemsResponse<ProjectThread>;
	return response.items ?? [];
}

export async function getThread(id: string, opts?: { fetch?: Fetcher }): Promise<ProjectThread> {
	const response = (await doGet(`/threads/${id}`, opts)) as ProjectThread;
	return response;
}

export async function getProject(projectID: string, opts?: { fetch?: Fetcher }): Promise<Project> {
	const response = (await doGet(`/projects/${projectID}`, opts)) as Project;
	return response;
}

export async function listTasks(opts?: { fetch?: Fetcher }): Promise<ProjectTask[]> {
	const response = (await doGet('/tasks', opts)) as ItemsResponse<ProjectTask>;
	return response.items ?? [];
}

export async function getTask(taskID: string, opts?: { fetch?: Fetcher }): Promise<ProjectTask> {
	const response = (await doGet(`/tasks/${taskID}`, opts)) as ProjectTask;
	return response;
}

export async function listModelProviders(opts?: { fetch?: Fetcher }): Promise<ModelProvider[]> {
	const response = (await doGet('/model-providers', opts)) as ItemsResponse<ModelProvider>;
	return response.items ?? [];
}

export async function getModelProvider(
	providerID: string,
	opts?: { fetch?: Fetcher }
): Promise<ModelProvider> {
	const response = (await doGet(`/model-providers/${providerID}`, opts)) as ModelProvider;
	return response;
}

export async function revealModelProvider(
	providerID: string,
	opts?: { fetch?: Fetcher }
): Promise<Record<string, string> | undefined> {
	const response = (await doPost(
		`/model-providers/${providerID}/reveal`,
		{},
		{
			...opts,
			dontLogErrors: true
		}
	)) as Record<string, string> | undefined;
	return response;
}

export async function configureModelProvider(
	providerID: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/model-providers/${providerID}/configure`, envs, opts);
}

export async function deconfigureModelProvider(
	providerID: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/model-providers/${providerID}/deconfigure`, {}, opts);
}

export async function validateModelProvider(
	providerID: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/model-providers/${providerID}/validate`, envs, {
		...opts,
		dontLogErrors: true
	});
}

export async function listModels(opts?: { fetch?: Fetcher }): Promise<Model[]> {
	const response = (await doGet('/models', opts)) as ItemsResponse<Model>;
	return response.items ?? [];
}

export async function updateModel(modelID: string, model: Model): Promise<void> {
	await doPut(`/models/${modelID}`, model);
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

export async function listDefaultModelAliases(opts?: {
	fetch?: Fetcher;
}): Promise<DefaultModelAlias[]> {
	const response = (await doGet(
		'/default-model-aliases',
		opts
	)) as ItemsResponse<DefaultModelAlias>;
	return response.items ?? [];
}

export async function updateDefaultModelAlias(
	alias: ModelAlias,
	defaultModelAlias: DefaultModelAlias
): Promise<void> {
	await doPut(`/default-model-aliases/${alias}`, defaultModelAlias);
}

export async function listAccessControlRules(opts?: {
	fetch?: Fetcher;
}): Promise<AccessControlRule[]> {
	const response = (await doGet(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/access-control-rules`,
		opts
	)) as ItemsResponse<AccessControlRule>;
	return response.items ?? [];
}

export async function getAccessControlRule(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<AccessControlRule> {
	const response = (await doGet(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/access-control-rules/${id}`,
		opts
	)) as AccessControlRule;
	return response;
}

export async function createAccessControlRule(
	rule: AccessControlRuleManifest
): Promise<AccessControlRule> {
	const response = (await doPost(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/access-control-rules`,
		rule
	)) as AccessControlRule;
	return response;
}

export async function updateAccessControlRule(
	id: string,
	rule: AccessControlRuleManifest
): Promise<AccessControlRule> {
	return (await doPut(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/access-control-rules/${id}`,
		rule
	)) as AccessControlRule;
}

export async function deleteAccessControlRule(id: string): Promise<void> {
	await doDelete(`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/access-control-rules/${id}`);
}

export async function listAuthProviders(opts?: { fetch?: Fetcher }): Promise<AuthProvider[]> {
	const list = (await doGet('/auth-providers', opts)) as ItemsResponse<AuthProvider>;
	return list.items ?? [];
}

export async function configureAuthProvider(
	authProviderID: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/auth-providers/${authProviderID}/configure`, envs, opts);
}

export async function revealAuthProvider(
	authProviderID: string,
	opts?: { fetch?: Fetcher }
): Promise<Record<string, string> | undefined> {
	const response = (await doPost(
		`/auth-providers/${authProviderID}/reveal`,
		{},
		{
			...opts,
			dontLogErrors: true
		}
	)) as Record<string, string> | undefined;
	return response;
}

export async function deconfigureAuthProvider(
	authProviderID: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/auth-providers/${authProviderID}/deconfigure`, {}, opts);
}

export async function getBootstrapStatus(): Promise<BootstrapStatus> {
	return (await doGet('/bootstrap')) as BootstrapStatus;
}

export async function bootstrapLogin(token: string) {
	const response = (await doPost(
		'/bootstrap/login',
		{},
		{
			headers: {
				Authorization: `Bearer ${token}`
			}
		}
	)) as BootstrapStatus;
	return response;
}

export async function bootstrapLogout() {
	return doPost('/bootstrap/logout', {});
}

function camelToSnakeCase(str: string): string {
	return str.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`);
}

function buildQueryString(
	filters: Record<string, string | number | boolean | string[] | undefined | null>
) {
	return Object.entries(filters)
		.filter(([_, value]) => value !== undefined && value !== null)
		.map(([key, value]) => {
			if (Array.isArray(value)) {
				// Join arrays with commas for multi-value parameters
				return `${camelToSnakeCase(key)}=${encodeURIComponent(value.join(','))}`;
			}
			return `${camelToSnakeCase(key)}=${typeof value === 'string' ? encodeURIComponent(value) : value}`;
		})
		.join('&');
}

export async function listAuditLogs(filters?: AuditLogURLFilters, opts?: { fetch?: Fetcher }) {
	const queryString = buildQueryString(filters ?? {});
	const response = (await doGet(
		`/mcp-audit-logs${queryString ? `?${queryString}` : ''}`,
		opts
	)) as PaginatedResponse<AuditLog>;
	return response;
}

export async function listServerOrInstanceAuditLogs(
	mcpId: string, // can either by server instance or mcp server id ex. ms- or msi-
	filters?: AuditLogURLFilters,
	opts?: { fetch?: Fetcher }
) {
	const queryString = buildQueryString(filters ?? {});
	const response = (await doGet(
		`/mcp-audit-logs/${mcpId}${queryString ? `?${queryString}` : ''}`,
		opts
	)) as PaginatedResponse<AuditLog>;
	return response;
}

type AuditLogUsageFilters = {
	mcp_id?: string;
	mcp_server_catalog_entry_names?: string;
	mcp_server_display_names?: string;
	user_ids?: string;
	start_time?: string | null;
	end_time?: string | null;
};

export async function listAuditLogUsageStats(
	filters?: Partial<AuditLogUsageFilters>,
	opts?: { fetch?: Fetcher }
) {
	const queryString = buildQueryString(filters ?? {});
	const response = (await doGet(
		`/mcp-stats${queryString ? `?${queryString}` : ''}`,
		opts
	)) as AuditLogUsageStats;
	return response;
}

export async function listAuditLogFilterOptions(filterId: string, opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/mcp-audit-logs/filter-options/${filterId}`, opts)) as {
		options: string[];
	};
	return response;
}

type ServerOrInstanceAuditLogStatsFilters = {
	start_time?: string;
	end_time?: string;
};
export async function listServerOrInstanceAuditLogStats(
	mcpId: string, // can either by server instance or mcp server id ex. ms- or msi-
	filters?: ServerOrInstanceAuditLogStatsFilters,
	opts?: { fetch?: Fetcher }
) {
	const queryString = buildQueryString(filters ?? {});
	const response = (await doGet(
		`/mcp-stats/${mcpId}${queryString ? `?${queryString}` : ''}`,
		opts
	)) as AuditLogUsageStats;
	return response;
}

export async function getK8sServerDetail(mcpServerId: string, opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/mcp-servers/${mcpServerId}/details`, opts)) as K8sServerDetail;
	return response;
}

export async function restartK8sDeployment(mcpServerId: string, opts?: { fetch?: Fetcher }) {
	await doPost(`/mcp-servers/${mcpServerId}/restart`, {}, opts);
}

export async function getDefaultBaseAgent(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/agents', opts)) as ItemsResponse<BaseAgent>;
	return response.items?.find((agent) => agent.default);
}

export async function updateBaseAgent(agent: BaseAgent, opts?: { fetch?: Fetcher }) {
	return (await doPut(`/agents/${agent.id}`, agent, opts)) as BaseAgent;
}

export async function listMCPFilters(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/mcp-webhook-validations', opts)) as ItemsResponse<MCPFilter>;
	return response.items ?? [];
}

export async function getMCPFilter(id: string, opts?: { fetch?: Fetcher }) {
	return (await doGet(`/mcp-webhook-validations/${id}`, opts)) as MCPFilter;
}

export async function deleteMCPFilter(id: string) {
	await doDelete(`/mcp-webhook-validations/${id}`);
}

export async function createMCPFilter(filter: MCPFilterManifest, opts?: { fetch?: Fetcher }) {
	return (await doPost('/mcp-webhook-validations', filter, opts)) as MCPFilter;
}

export async function updateMCPFilter(
	id: string,
	filter: MCPFilterManifest,
	opts?: { fetch?: Fetcher }
) {
	return (await doPut(`/mcp-webhook-validations/${id}`, filter, opts)) as MCPFilter;
}

export async function removeSecret(id: string) {
	await doDelete(`/mcp-webhook-validations/${id}/secret`);
}

export async function listCatalogCategories(catalogId: string, opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/mcp-catalogs/${catalogId}/categories`, opts)) as string[];
	return response;
}

export async function listAllUserWorkspaceCatalogEntries(opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/workspaces/all-entries`, opts)) as ItemsResponse<MCPCatalogEntry>;
	return (
		response.items?.map((item) => {
			return {
				...item,
				isCatalogEntry: true
			};
		}) ?? []
	);
}

export async function listAllUserWorkspaceMCPServers(opts?: { fetch?: Fetcher }) {
	const response = (await doGet(
		`/workspaces/all-servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function listAllUserWorkspaceAccessControlRules(opts?: { fetch?: Fetcher }) {
	const response = (await doGet(
		`/workspaces/all-access-control-rules`,
		opts
	)) as ItemsResponse<AccessControlRule>;
	return response.items ?? [];
}

export async function updateDefaultUsersRoleSettings(role: number, opts?: { fetch?: Fetcher }) {
	await doPost('/user-default-role-settings', { role }, opts);
}

export async function getDefaultUsersRoleSettings(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/user-default-role-settings', opts)) as { role: number };
	return response.role;
}
