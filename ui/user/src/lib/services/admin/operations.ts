import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import type { Skill } from '$lib/services/nanobot/types';
import type {
	ModelProvider,
	MCPCatalogServer,
	MCPServerInstance,
	MCPServerTool,
	Model,
	ModelAlias,
	DefaultModelAlias
} from '../chat/types';
import { doDelete, doGet, doPatch, doPost, doPut, handleResponse, type Fetcher } from '../http';
import type {
	FileScannerConfig,
	FileScannerProvider,
	MCPCatalog,
	MCPCatalogEntry,
	MCPCatalogEntryServerManifest,
	MCPCatalogManifest,
	OrgUser,
	OrgGroup,
	MCPCatalogServerManifest,
	AccessControlRule,
	AccessControlRuleManifest,
	ModelAccessPolicy,
	ModelAccessPolicyManifest,
	AuthProvider,
	BootstrapStatus,
	AuditLog,
	AuditLogUsageStats,
	AuditLogURLFilters,
	K8sServerDetail,
	MCPFilter,
	MCPFilterManifest,
	TempUser,
	ScheduledAuditLogExport,
	StorageCredentials,
	AuditLogExport,
	AuditLogExportInput,
	ScheduledAuditLogExportInput,
	K8sSettings,
	ServerK8sSettings,
	MCPCompositeDeletionDependency,
	AppPreferences,
	GroupRoleAssignment,
	GroupRoleAssignmentList,
	MCPCapacityInfo,
	MCPServerOAuthCredentialRequest,
	MCPServerOAuthCredentialStatus,
	TokenUsageTimeRange,
	TotalTokenUsage,
	TokenUsage,
	SkillRepository,
	SkillRepositoryManifest,
	SkillAccessPolicy,
	SkillAccessPolicyManifest,
	MessagePolicy,
	MessagePolicyManifest,
	MessagePolicyViolation,
	MessagePolicyViolationFilters,
	MessagePolicyViolationStats,
	RestartNanobotAgentDeploymentsResult,
	SystemMCPCatalog,
	SystemMCPCatalogManifest,
	SystemMCPServer,
	SystemMCPServerCatalogEntry,
	SystemMCPServerCatalogEntryManifest,
	SystemMCPServerManifest,
	DeviceMCPServerOccurrenceResponse,
	DeviceMCPServerDetail,
	DeviceScan,
	DeviceScanListFilters,
	DeviceScanResponse,
	DeviceScanStats,
	DeviceSkillListFilters,
	DeviceSkillOccurrenceResponse,
	DeviceSkillDetail,
	DeviceSkillStatResponse,
	DeviceClientFleetSummary,
	DeviceClientFleetSummaryResponse,
	DeviceClientListFilters,
	OAuthDebuggerAuthorizationURL,
	OAuthDebuggerAuthorizationURLRequest,
	OAuthDebuggerRegisterClientResponse,
	OAuthDebuggerTokenRequest,
	OAuthToken
} from './types';
import { MCPCompositeDeletionDependencyError } from './types';

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
	opts?: { fetch?: Fetcher; all?: boolean }
): Promise<MCPCatalogEntry[]> {
	const url = opts?.all
		? `/mcp-catalogs/${catalogID}/entries?all=true`
		: `/mcp-catalogs/${catalogID}/entries`;
	const response = (await doGet(url, opts)) as ItemsResponse<MCPCatalogEntry>;
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
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
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

export async function getMCPCatalogEntryServerK8sSettingsStatus(
	entryID: string,
	serverID: string,
	opts?: { dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/entries/${entryID}/servers/${serverID}/k8s-settings-status`,
		opts
	)) as ServerK8sSettings;
	return response;
}

export async function redeployMCPCatalogServerWithK8sSettings(
	entryID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/entries/${entryID}/servers/${serverID}/redeploy-with-k8s-settings`,
		{},
		opts
	);
	return response;
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
	await doDelete(`/mcp-catalogs/${catalogID}/servers/${serverID}`, {
		responseHandler: async (resp, path, opts) => {
			if (resp.status === 409 && resp.headers.get('Content-Type')?.includes('application/json')) {
				const body = (await resp.json()) as {
					message?: string;
					dependencies: MCPCompositeDeletionDependency[];
				};

				if (body.dependencies && body.dependencies.length > 0) {
					throw new MCPCompositeDeletionDependencyError(
						body.message ??
							'All dependencies on this MCP server must be removed before it can be deleted',
						body.dependencies
					);
				}
			}

			return handleResponse(resp, path, opts);
		}
	});
}

export async function listMCPCatalogServers(
	catalogID: string,
	opts?: { fetch?: Fetcher; all?: boolean }
): Promise<MCPCatalogServer[]> {
	const url = opts?.all
		? `/mcp-catalogs/${catalogID}/servers?all=true`
		: `/mcp-catalogs/${catalogID}/servers`;
	const response = (await doGet(url, opts)) as ItemsResponse<MCPCatalogServer>;
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
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<MCPCatalogServer> {
	const response = (await doGet(
		`/mcp-catalogs/${catalogID}/servers/${serverID}`,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function getMCPServerById(
	serverID: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<MCPCatalogServer> {
	const response = (await doGet(`/mcp-servers/${serverID}`, opts)) as MCPCatalogServer;
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
	opts?: { fetch?: Fetcher; dryRun?: boolean }
): Promise<MCPCatalogEntry | void> {
	const path = `/mcp-catalogs/${catalogID}/entries/${entryID}/generate-tool-previews`;
	const url = opts?.dryRun ? `${path}?dryRun=true` : path;
	const resp = await doPost(url, body ?? {}, {
		...opts,
		dontLogErrors: true
	});
	return opts?.dryRun ? (resp as MCPCatalogEntry) : undefined;
}

export async function generateMcpCompositeComponentToolPreviews(
	catalogID: string,
	compositeEntryID: string,
	componentID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
	},
	opts?: { fetch?: Fetcher; dryRun?: boolean }
): Promise<MCPCatalogEntry | void> {
	const path = `/mcp-catalogs/${catalogID}/entries/${compositeEntryID}/${componentID}/generate-tool-previews`;
	const url = opts?.dryRun ? `${path}?dryRun=true` : path;
	const resp = await doPost(url, body ?? {}, {
		...opts,
		dontLogErrors: true
	});
	return opts?.dryRun ? (resp as MCPCatalogEntry) : undefined;
}

export async function getMcpCatalogToolPreviewsOauth(
	catalogID: string,
	entryID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
		componentConfigs?: Record<
			string,
			{
				config?: Record<string, string>;
				url?: string;
				skip?: boolean;
			}
		>;
	},
	opts?: { fetch?: Fetcher; dryRun?: boolean }
): Promise<string | Record<string, string>> {
	try {
		const path = `/mcp-catalogs/${catalogID}/entries/${entryID}/generate-tool-previews/oauth-url`;
		const url = opts?.dryRun ? `${path}?dryRun=true` : path;
		const response = (await doPost(url, body ?? {}, {
			...opts,
			dontLogErrors: true
		})) as
			| {
					oauthURL: string;
			  }
			| Record<string, string>;

		// Check if response has oauthURL property (single server response)
		if (response && typeof response === 'object' && 'oauthURL' in response) {
			return response.oauthURL;
		}

		// Otherwise it's a map of component IDs to OAuth URLs
		return response as Record<string, string>;
	} catch (_err) {
		return '';
	}
}

export async function getMcpCompositeComponentToolPreviewsOauth(
	catalogID: string,
	compositeEntryID: string,
	componentID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
	},
	opts?: { fetch?: Fetcher; dryRun?: boolean }
): Promise<string> {
	try {
		const path = `/mcp-catalogs/${catalogID}/entries/${compositeEntryID}/${componentID}/generate-tool-previews/oauth-url`;
		const url = opts?.dryRun ? `${path}?dryRun=true` : path;
		const response = (await doPost(url, body ?? {}, {
			...opts,
			dontLogErrors: true
		})) as {
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

export async function listUsersIncludeDeleted(opts?: {
	fetch?: Fetcher;
	signal?: AbortSignal;
}): Promise<OrgUser[]> {
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
	const params: string[] = [];
	if (opts?.query !== undefined) {
		params.push(`name=${encodeURIComponent(opts.query)}`);
	}
	const queryString = params.length ? `?${params.join('&')}` : '';
	const response = (await doGet(`/groups${queryString}`, opts)) as OrgGroup[];
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

export async function listModels(opts?: { fetch?: Fetcher; all?: boolean }): Promise<Model[]> {
	const url = opts?.all ? '/models?all=true' : '/models';
	const response = (await doGet(url, opts)) as ItemsResponse<Model>;
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

// Model Permission Rules
export async function listModelAccessPolicies(opts?: {
	fetch?: Fetcher;
}): Promise<ModelAccessPolicy[]> {
	const response = (await doGet(
		'/model-access-policies',
		opts
	)) as ItemsResponse<ModelAccessPolicy>;
	return response.items ?? [];
}

export async function getModelAccessPolicy(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<ModelAccessPolicy> {
	const response = (await doGet(`/model-access-policies/${id}`, opts)) as ModelAccessPolicy;
	return response;
}

export async function createModelAccessPolicy(
	rule: ModelAccessPolicyManifest
): Promise<ModelAccessPolicy> {
	const response = (await doPost('/model-access-policies', rule)) as ModelAccessPolicy;
	return response;
}

export async function updateModelAccessPolicy(
	id: string,
	rule: ModelAccessPolicyManifest
): Promise<ModelAccessPolicy> {
	return (await doPut(`/model-access-policies/${id}`, rule)) as ModelAccessPolicy;
}

export async function deleteModelAccessPolicy(id: string): Promise<void> {
	await doDelete(`/model-access-policies/${id}`);
}

export async function listMessagePolicies(opts?: { fetch?: Fetcher }): Promise<MessagePolicy[]> {
	const response = (await doGet('/message-policies', opts)) as ItemsResponse<MessagePolicy>;
	return response.items ?? [];
}

export async function getMessagePolicy(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MessagePolicy> {
	return (await doGet(`/message-policies/${id}`, opts)) as MessagePolicy;
}

export async function createMessagePolicy(manifest: MessagePolicyManifest): Promise<MessagePolicy> {
	return (await doPost('/message-policies', manifest)) as MessagePolicy;
}

export async function updateMessagePolicy(
	id: string,
	manifest: MessagePolicyManifest
): Promise<MessagePolicy> {
	return (await doPut(`/message-policies/${id}`, manifest)) as MessagePolicy;
}

export async function deleteMessagePolicy(id: string): Promise<void> {
	await doDelete(`/message-policies/${id}`);
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

export async function getAuditLog(id: string | number, opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/mcp-audit-logs/detail/${id}`, opts)) as AuditLog;
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

export const AUDIT_LOG_FILTER_OPTIONS_LIMIT = 1000;

export async function listAuditLogFilterOptions(
	filterId: string,
	opts?: { fetch?: Fetcher } & Partial<AuditLogURLFilters>
) {
	const { fetch: fetchFn, ...filters } = opts ?? {};
	const queryString = buildQueryString({ ...filters, limit: AUDIT_LOG_FILTER_OPTIONS_LIMIT });
	const response = (await doGet(
		`/mcp-audit-logs/filter-options/${filterId}${queryString ? `?${queryString}` : ''}`,
		{ fetch: fetchFn }
	)) as {
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

export async function getK8sServerDetail(
	mcpServerId: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
) {
	const response = (await doGet(`/mcp-servers/${mcpServerId}/details`, opts)) as K8sServerDetail;
	return response;
}

export async function restartK8sDeployment(mcpServerId: string, opts?: { fetch?: Fetcher }) {
	await doPost(`/mcp-servers/${mcpServerId}/restart`, {}, opts);
}

export async function getMcpCatalogServerK8sSettingsStatus(
	mcpServerId: string,
	opts?: { dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/mcp-catalogs/${DEFAULT_MCP_CATALOG_ID}/servers/${mcpServerId}/k8s-settings-status`,
		opts
	)) as ServerK8sSettings;
	return response;
}

export async function redeployWithK8sSettings(
	mcpServerId: string,
	catalogId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost(
		`/mcp-catalogs/${catalogId}/servers/${mcpServerId}/redeploy-with-k8s-settings`,
		{},
		opts
	);
	return response;
}

export async function listMCPFilters(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/mcp-webhook-validations', opts)) as ItemsResponse<MCPFilter>;
	return response.items ?? [];
}

export async function getMCPFilter(id: string, opts?: { fetch?: Fetcher }) {
	return (await doGet(`/mcp-webhook-validations/${id}`, opts)) as MCPFilter;
}

export async function deleteMCPFilter(id: string, opts?: { keepalive?: boolean }) {
	await doDelete(`/mcp-webhook-validations/${id}`, {
		keepalive: opts?.keepalive,
		dontLogErrors: opts?.keepalive
	});
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

export async function configureMCPFilter(
	id: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<MCPFilter> {
	return (await doPost(`/mcp-webhook-validations/${id}/configure`, envs, opts)) as MCPFilter;
}

export async function deconfigureMCPFilter(id: string, opts?: { fetch?: Fetcher }): Promise<void> {
	await doPost(`/mcp-webhook-validations/${id}/deconfigure`, {}, opts);
}

export async function launchMCPFilter(id: string): Promise<{
	success: boolean;
	message?: string;
	code?: number;
}> {
	try {
		await doPost(`/mcp-webhook-validations/${id}/launch`, {}, { dontLogErrors: true });
		return {
			success: true
		};
	} catch (err) {
		if (err instanceof Error) {
			if (err.message.includes('404')) {
				return {
					success: false,
					message: err.message,
					code: 404
				};
			} else if (err.message.includes('503')) {
				return {
					success: false,
					message: err.message,
					code: 503
				};
			} else {
				return {
					success: false,
					message: err.message,
					code: 500
				};
			}
		}

		throw err;
	}
}

export async function revealMCPFilter(
	id: string,
	opts?: { dontLogErrors?: boolean }
): Promise<Record<string, string>> {
	return doPost(`/mcp-webhook-validations/${id}/reveal`, {}, opts) as Promise<
		Record<string, string>
	>;
}

export async function restartMCPFilter(id: string, opts?: { fetch?: Fetcher }): Promise<void> {
	await doPost(`/mcp-webhook-validations/${id}/restart`, {}, opts);
}

export async function getMCPFilterDetails(
	id: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
) {
	const response = (await doGet(`/mcp-webhook-validations/${id}/details`, opts)) as K8sServerDetail;
	return response;
}

export async function listCatalogCategories(catalogId: string, opts?: { fetch?: Fetcher }) {
	const response = (await doGet(`/mcp-catalogs/${catalogId}/categories`, opts)) as string[];
	return response;
}

export async function listAllCatalogDeployedSingleRemoteServers(
	catalogId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = (await doGet(
		`/mcp-catalogs/${catalogId}/entries/all-servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
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

export async function listAllWorkspaceDeployedSingleRemoteServers(opts?: { fetch?: Fetcher }) {
	const response = (await doGet(
		`/workspaces/all-entries/all-servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
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

export async function listExplicitRoleEmails(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/setup/explicit-role-emails', opts)) as {
		owners: string[] | null;
		admins: string[] | null;
	};
	return response;
}

export async function initiateTempLogin(authProviderName: string, authProviderNamespace?: string) {
	const response = (await doPost('/setup/initiate-temp-login', {
		authProviderName,
		authProviderNamespace
	})) as {
		redirectUrl: string;
		tokenId: string;
	};
	return response;
}

export async function getTempUser() {
	const response = (await doGet('/setup/temp-user')) as TempUser;
	return response;
}

export async function confirmTempUserAsOwner(email: string) {
	const response = (await doPost('/setup/confirm-owner', { email })) as {
		success: boolean;
		userId: number;
		email: string;
		message: string;
	};
	return response;
}

export async function cancelTempLogin() {
	await doPost(
		'/setup/cancel-temp-login',
		{},
		{
			dontLogErrors: true
		}
	);
}

export async function getAuditLogExports(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/audit-log-exports', opts)) as PaginatedResponse<AuditLogExport>;
	return response;
}

export async function getAuditLogExport(name: string, opts?: { fetch?: Fetcher }) {
	const response = await doGet(`/audit-log-exports/${name}`, opts);
	return response;
}

export async function createAuditLogExport(
	request: AuditLogExportInput,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost('/audit-log-exports', request, opts);
	return response;
}

export async function deleteAuditLogExport(name: string, opts?: { signal?: AbortSignal }) {
	await doDelete(`/audit-log-exports/${name}`, { signal: opts?.signal });
}

// Scheduled Audit Log Exports
export async function getScheduledAuditLogExports(opts?: { fetch?: Fetcher }) {
	const response = (await doGet(
		'/scheduled-audit-log-exports',
		opts
	)) as PaginatedResponse<ScheduledAuditLogExport>;
	return response;
}

export async function getScheduledAuditLogExport(
	name: string,
	opts?: { fetch?: Fetcher }
): Promise<ScheduledAuditLogExport> {
	const response = await doGet(`/scheduled-audit-log-exports/${name}`, opts);
	return response as ScheduledAuditLogExport;
}

export async function createScheduledAuditLogExport(
	request: ScheduledAuditLogExportInput,
	opts?: { dontLogErrors?: boolean }
) {
	const response = await doPost('/scheduled-audit-log-exports', request, opts);
	return response;
}

export async function updateScheduledAuditLogExport(
	id: string,
	request: Partial<ScheduledAuditLogExportInput>,
	opts?: { dontLogErrors?: boolean }
) {
	const response = await doPatch(`/scheduled-audit-log-exports/${id}`, request, opts);
	return response;
}

export async function deleteScheduledAuditLogExport(name: string, opts?: { signal?: AbortSignal }) {
	await doDelete(`/scheduled-audit-log-exports/${name}`, { signal: opts?.signal });
}

// Storage Credentials
export async function getStorageCredentials() {
	const response = (await doGet('/storage-credentials', {
		dontLogErrors: true
	})) as StorageCredentials;
	return response;
}

export async function configureStorageCredentials(
	request: StorageCredentials,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost('/storage-credentials', request, opts);
	return response;
}

export async function deleteStorageCredentials(
	opts?:
		| {
				signal?: AbortSignal | undefined;
		  }
		| undefined
) {
	const response = await doDelete('/storage-credentials', opts);
	return response;
}

export async function testStorageCredentials(
	request: StorageCredentials,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost('/storage-credentials/test', request, opts);
	return response;
}

export async function getMCPServer(
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doGet(`/mcp-servers/${serverID}`, opts)) as MCPCatalogServer;
	return response;
}

export async function refreshCompositeComponents(
	catalogID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/entries/${entryID}/refresh-components`,
		{},
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function listK8sSettings(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/k8s-settings', opts)) as K8sSettings;
	return response;
}

export async function updateK8sSettings(settings: K8sSettings, opts?: { fetch?: Fetcher }) {
	return (await doPut('/k8s-settings', settings, opts)) as K8sSettings;
}

export async function getEula() {
	const response = (await doGet('/eula', {
		dontLogErrors: true
	})) as {
		accepted: boolean;
	};
	return response;
}

export async function acceptEula() {
	return (await doPut('/eula', {
		accepted: true
	})) as {
		accepted: boolean;
	};
}

export async function listAppPreferences(opts?: { fetch?: Fetcher }) {
	const response = (await doGet('/app-preferences', opts)) as AppPreferences;
	return response;
}

export async function updateAppPreferences(
	preferences: AppPreferences,
	opts?: { fetch?: Fetcher }
) {
	return (await doPut('/app-preferences', preferences, opts)) as AppPreferences;
}

export async function listGroupRoleAssignments(opts?: {
	fetch?: Fetcher;
}): Promise<GroupRoleAssignment[]> {
	const response = (await doGet('/group-role-assignments', opts)) as GroupRoleAssignmentList;
	return response.items ?? [];
}

export async function getGroupRoleAssignment(
	groupName: string,
	opts?: { fetch?: Fetcher }
): Promise<GroupRoleAssignment> {
	const response = (await doGet(
		`/group-role-assignments/${encodeURIComponent(groupName)}`,
		opts
	)) as GroupRoleAssignment;
	return response;
}

export async function createGroupRoleAssignment(
	assignment: GroupRoleAssignment,
	opts?: { fetch?: Fetcher }
): Promise<GroupRoleAssignment> {
	const response = (await doPost(
		'/group-role-assignments',
		assignment,
		opts
	)) as GroupRoleAssignment;
	return response;
}

export async function updateGroupRoleAssignment(
	groupName: string,
	assignment: GroupRoleAssignment,
	opts?: { fetch?: Fetcher }
): Promise<GroupRoleAssignment> {
	const response = (await doPut(
		`/group-role-assignments/${encodeURIComponent(groupName)}`,
		assignment,
		opts
	)) as GroupRoleAssignment;
	return response;
}

export async function deleteGroupRoleAssignment(
	groupName: string,
	opts?: { signal?: AbortSignal | undefined }
): Promise<void> {
	await doDelete(`/group-role-assignments/${encodeURIComponent(groupName)}`, opts);
}

// MCP Capacity
export async function getMCPCapacity(opts?: { fetch?: Fetcher }): Promise<MCPCapacityInfo> {
	const response = (await doGet('/mcp-capacity', opts)) as MCPCapacityInfo;
	return response;
}

// GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
export async function getMCPCatalogEntryOAuthCredentials(
	catalogID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerOAuthCredentialStatus> {
	const response = (await doGet(`/mcp-catalogs/${catalogID}/entries/${entryID}/oauth-credentials`, {
		...opts,
		dontLogErrors: true
	})) as MCPServerOAuthCredentialStatus;
	return response;
}

// POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
export async function setMCPCatalogEntryOAuthCredentials(
	catalogID: string,
	entryID: string,
	credentials: MCPServerOAuthCredentialRequest,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerOAuthCredentialStatus> {
	const response = (await doPost(
		`/mcp-catalogs/${catalogID}/entries/${entryID}/oauth-credentials`,
		credentials,
		opts
	)) as MCPServerOAuthCredentialStatus;
	return response;
}

// DELETE /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
export async function deleteMCPCatalogEntryOAuthCredentials(
	catalogID: string,
	entryID: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/mcp-catalogs/${catalogID}/entries/${entryID}/oauth-credentials`, opts);
}

export async function listTotalTokenUsage(opts?: { fetch?: Fetcher }) {
	const response = await doGet('/total-token-usage', opts);
	return response as TotalTokenUsage;
}

function formatTokenUsageDate(d: Date | string): string {
	return typeof d === 'string' ? d : d.toISOString();
}

function tokenUsageQueryString(timeRange: TokenUsageTimeRange): string {
	const parts = [
		`start=${encodeURIComponent(formatTokenUsageDate(timeRange.start))}`,
		`end=${encodeURIComponent(formatTokenUsageDate(timeRange.end))}`
	];
	return parts.join('&');
}

/** Returns token usage for all users in the time range as a flat list. Does not include personal token. */
export async function listTokenUsage(
	timeRange: TokenUsageTimeRange,
	opts?: { fetch?: Fetcher; signal?: AbortSignal }
): Promise<TokenUsage[]> {
	const queryString = tokenUsageQueryString(timeRange);
	const response = await doGet(`/token-usage?${queryString}`, opts);
	return unwrapTokenUsageList(response);
}

export async function listRemainingTokenUsageForUser(userId: string, opts?: { fetch?: Fetcher }) {
	const response = await doGet(`/users/${userId}/remaining-token-usage`, opts);
	return response;
}

export async function listTotalTokenUsageForUser(userId: string, opts?: { fetch?: Fetcher }) {
	const response = await doGet(`/users/${userId}/total-token-usage`, opts);
	return response;
}

export async function listTokenUsageForUser(
	userId: string,
	timeRange: TokenUsageTimeRange,
	opts?: { fetch?: Fetcher }
): Promise<TokenUsage[]> {
	const queryString = tokenUsageQueryString(timeRange);
	const response = await doGet(`/users/${userId}/token-usage?${queryString}`, opts);
	return unwrapTokenUsageList(response);
}

function unwrapTokenUsageList(response: unknown): TokenUsage[] {
	if (Array.isArray(response)) return response;
	const list = response as { items?: TokenUsage[] };
	return list?.items ?? [];
}

export async function listSkillRepositories(opts?: {
	fetch?: Fetcher;
}): Promise<SkillRepository[]> {
	const response = (await doGet('/skill-repositories', opts)) as ItemsResponse<SkillRepository>;
	return response.items ?? [];
}

export async function getSkillRepository(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<SkillRepository> {
	const response = (await doGet(`/skill-repositories/${id}`, opts)) as SkillRepository;
	return response;
}

export async function createSkillRepository(
	request: SkillRepositoryManifest,
	opts?: { fetch?: Fetcher }
): Promise<SkillRepository> {
	const response = (await doPost('/skill-repositories', request, opts)) as SkillRepository;
	return response;
}

export async function updateSkillRepository(
	id: string,
	request: SkillRepositoryManifest,
	opts?: { fetch?: Fetcher }
): Promise<SkillRepository> {
	const response = (await doPut(`/skill-repositories/${id}`, request, opts)) as SkillRepository;
	return response;
}

export async function deleteSkillRepository(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/skill-repositories/${id}`, opts);
}

export async function refreshSkillRepository(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/skill-repositories/${id}/refresh`, {}, opts);
}

export async function listAllSkills(opts?: {
	fetch?: Fetcher;
	query?: string;
	repoId?: string;
	limit?: number;
}): Promise<Skill[]> {
	const params = new URLSearchParams();
	params.set('all', 'true');
	params.set('limit', String(opts?.limit ?? 200));
	if (opts?.query != null) params.set('q', opts.query);
	if (opts?.repoId != null) params.set('repoID', opts.repoId);
	const queryString = params.toString();
	const url = queryString ? `/skills?${queryString}` : '/skills';
	const response = (await doGet(url, opts)) as ItemsResponse<Skill>;
	return response.items ?? [];
}

export async function listSkillAccessPolicies(opts?: {
	fetch?: Fetcher;
}): Promise<SkillAccessPolicy[]> {
	const response = (await doGet('/skill-access-rules', opts)) as ItemsResponse<SkillAccessPolicy>;
	return response.items ?? [];
}

export async function getSkillAccessPolicy(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<SkillAccessPolicy> {
	const response = (await doGet(`/skill-access-rules/${id}`, opts)) as SkillAccessPolicy;
	return response;
}

export async function createSkillAccessPolicy(
	request: SkillAccessPolicyManifest,
	opts?: { fetch?: Fetcher }
): Promise<SkillAccessPolicy> {
	const response = (await doPost('/skill-access-rules', request, opts)) as SkillAccessPolicy;
	return response;
}

export async function updateSkillAccessPolicy(
	id: string,
	request: SkillAccessPolicyManifest,
	opts?: { fetch?: Fetcher }
): Promise<SkillAccessPolicy> {
	const response = (await doPut(`/skill-access-rules/${id}`, request, opts)) as SkillAccessPolicy;
	return response;
}

export async function deleteSkillAccessPolicy(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/skill-access-rules/${id}`, opts);
}

// Message Policy Violations

function buildMessagePolicyViolationParams(filters?: MessagePolicyViolationFilters): string {
	if (!filters) return '';
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(filters)) {
		if (value != null && value !== '') {
			params.set(key, String(value));
		}
	}
	const str = params.toString();
	return str ? `?${str}` : '';
}

export async function listMessagePolicyViolations(
	filters?: MessagePolicyViolationFilters,
	opts?: { fetch?: Fetcher }
): Promise<PaginatedResponse<MessagePolicyViolation>> {
	return (await doGet(
		`/message-policy-violations${buildMessagePolicyViolationParams(filters)}`,
		opts
	)) as PaginatedResponse<MessagePolicyViolation>;
}

export async function getMessagePolicyViolation(
	id: number | string,
	opts?: { fetch?: Fetcher }
): Promise<MessagePolicyViolation> {
	return (await doGet(`/message-policy-violations/${id}`, opts)) as MessagePolicyViolation;
}

export async function listMessagePolicyViolationFilterOptions(
	filter: string,
	filters?: MessagePolicyViolationFilters,
	opts?: { fetch?: Fetcher }
): Promise<string[]> {
	const response = (await doGet(
		`/message-policy-violations/filter-options/${filter}${buildMessagePolicyViolationParams(filters)}`,
		opts
	)) as { options: string[] };
	return response.options ?? [];
}

export async function getMessagePolicyViolationStats(
	filters?: MessagePolicyViolationFilters,
	opts?: { fetch?: Fetcher }
): Promise<MessagePolicyViolationStats> {
	return (await doGet(
		`/message-policy-violation-stats${buildMessagePolicyViolationParams(filters)}`,
		opts
	)) as MessagePolicyViolationStats;
}

export async function listSystemMCPCatalogs(opts?: {
	fetch?: Fetcher;
}): Promise<SystemMCPCatalog[]> {
	const response = (await doGet('/system-mcp-catalogs', opts)) as ItemsResponse<SystemMCPCatalog>;
	return response.items ?? [];
}

export async function getSystemMCPCatalog(
	catalogId: string,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPCatalog> {
	return (await doGet(`/system-mcp-catalogs/${catalogId}`, opts)) as SystemMCPCatalog;
}

export async function createSystemMCPCatalog(
	manifest: SystemMCPCatalogManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPCatalog> {
	return (await doPost('/system-mcp-catalogs', manifest, opts)) as SystemMCPCatalog;
}

export async function updateSystemMCPCatalog(
	catalogId: string,
	manifest: SystemMCPCatalogManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPCatalog> {
	return (await doPut(`/system-mcp-catalogs/${catalogId}`, manifest, opts)) as SystemMCPCatalog;
}

export async function deleteSystemMCPCatalog(
	catalogId: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/system-mcp-catalogs/${catalogId}`, opts);
}

export async function refreshSystemMCPCatalog(
	catalogId: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/system-mcp-catalogs/${catalogId}/refresh`, {}, opts);
}

export async function listSystemMCPCatalogEntries(
	catalogId: string,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServerCatalogEntry[]> {
	const response = (await doGet(
		`/system-mcp-catalogs/${catalogId}/entries`,
		opts
	)) as ItemsResponse<SystemMCPServerCatalogEntry>;
	return response.items ?? [];
}

export async function createSystemMCPCatalogEntry(
	catalogId: string,
	manifest: SystemMCPServerCatalogEntryManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServerCatalogEntry> {
	return (await doPost(
		`/system-mcp-catalogs/${catalogId}/entries`,
		manifest,
		opts
	)) as SystemMCPServerCatalogEntry;
}

export async function getSystemMCPCatalogEntry(
	catalogId: string,
	entryId: string,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServerCatalogEntry> {
	return (await doGet(
		`/system-mcp-catalogs/${catalogId}/entries/${entryId}`,
		opts
	)) as SystemMCPServerCatalogEntry;
}

export async function updateSystemMCPCatalogEntry(
	catalogId: string,
	entryId: string,
	manifest: SystemMCPServerCatalogEntryManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServerCatalogEntry> {
	return (await doPut(
		`/system-mcp-catalogs/${catalogId}/entries/${entryId}`,
		manifest,
		opts
	)) as SystemMCPServerCatalogEntry;
}

export async function deleteSystemMCPCatalogEntry(
	catalogId: string,
	entryId: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/system-mcp-catalogs/${catalogId}/entries/${entryId}`, opts);
}

export async function listSystemMCPServers(opts?: { fetch?: Fetcher }): Promise<SystemMCPServer[]> {
	const response = (await doGet('/system-mcp-servers', opts)) as ItemsResponse<SystemMCPServer>;
	return response.items ?? [];
}

export async function getSystemMCPServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServer> {
	return (await doGet(`/system-mcp-servers/${id}`, opts)) as SystemMCPServer;
}

export async function createSystemMCPServer(
	manifest: SystemMCPServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServer> {
	return (await doPost('/system-mcp-servers', manifest, opts)) as SystemMCPServer;
}

export async function updateSystemMCPServer(
	id: string,
	manifest: SystemMCPServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServer> {
	return (await doPut(`/system-mcp-servers/${id}`, manifest, opts)) as SystemMCPServer;
}

export async function deleteSystemMCPServer(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/system-mcp-servers/${id}`, opts);
}

export async function configureSystemMCPServer(
	id: string,
	envVars: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServer> {
	return (await doPost(`/system-mcp-servers/${id}/configure`, envVars, opts)) as SystemMCPServer;
}

export async function deconfigureSystemMCPServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<SystemMCPServer> {
	return (await doPost(`/system-mcp-servers/${id}/deconfigure`, {}, opts)) as SystemMCPServer;
}

export async function restartSystemMCPServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/system-mcp-servers/${id}/restart`, {}, opts);
}

export async function revealSystemMCPServerCredentials(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<Record<string, string>> {
	return (await doPost(`/system-mcp-servers/${id}/reveal`, {}, opts)) as Record<string, string>;
}

export async function getSystemMCPServerDetails(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<K8sServerDetail> {
	return (await doGet(`/system-mcp-servers/${id}/details`, opts)) as K8sServerDetail;
}

export async function getSystemMCPServerTools(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerTool[]> {
	return (await doGet(`/system-mcp-servers/${id}/tools`, opts)) as MCPServerTool[];
}

// Device scans

export async function listDeviceScans(
	filters?: DeviceScanListFilters,
	opts?: { fetch?: Fetcher }
): Promise<DeviceScanResponse> {
	const queryString = buildQueryString(filters ?? {});
	return (await doGet(
		`/devices/scans${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceScanResponse;
}

export async function getDeviceScan(
	id: number | string,
	opts?: { fetch?: Fetcher }
): Promise<DeviceScan> {
	return (await doGet(`/devices/scans/${id}`, opts)) as DeviceScan;
}

export async function deleteDeviceScan(id: number | string): Promise<void> {
	await doDelete(`/devices/scans/${id}`);
}

export async function getDeviceMCPServerDetail(
	configHash: string,
	opts?: { fetch?: Fetcher }
): Promise<DeviceMCPServerDetail> {
	return (await doGet(
		`/devices/mcp-servers/${encodeURIComponent(configHash)}`,
		opts
	)) as DeviceMCPServerDetail;
}

export async function listDeviceMCPServerOccurrences(
	configHash: string,
	page: { limit?: number; offset?: number },
	opts?: { fetch?: Fetcher }
): Promise<DeviceMCPServerOccurrenceResponse> {
	const queryString = buildQueryString(page ?? {});
	return (await doGet(
		`/devices/mcp-servers/${encodeURIComponent(configHash)}/occurrences${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceMCPServerOccurrenceResponse;
}

export async function getDeviceScanStats(
	range?: { start?: string; end?: string },
	opts?: { fetch?: Fetcher }
): Promise<DeviceScanStats> {
	const queryString = buildQueryString(range ?? {});
	return (await doGet(
		`/devices/scan-stats${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceScanStats;
}

export async function listDeviceSkills(
	filters?: DeviceSkillListFilters,
	opts?: { fetch?: Fetcher }
): Promise<DeviceSkillStatResponse> {
	const queryString = buildQueryString(filters ?? {});
	return (await doGet(
		`/devices/skills${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceSkillStatResponse;
}

export async function getDeviceSkillDetail(
	name: string,
	opts?: { fetch?: Fetcher }
): Promise<DeviceSkillDetail> {
	return (await doGet(`/devices/skills/${encodeURIComponent(name)}`, opts)) as DeviceSkillDetail;
}

export async function listDeviceClients(
	filters?: DeviceClientListFilters,
	opts?: { fetch?: Fetcher }
): Promise<DeviceClientFleetSummaryResponse> {
	const queryString = buildQueryString(filters ?? {});
	return (await doGet(
		`/devices/clients${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceClientFleetSummaryResponse;
}

export async function getDeviceClient(
	name: string,
	opts?: { fetch?: Fetcher }
): Promise<DeviceClientFleetSummary> {
	return (await doGet(
		`/devices/clients/${encodeURIComponent(name)}`,
		opts
	)) as DeviceClientFleetSummary;
}

export async function listDeviceSkillOccurrences(
	name: string,
	page: { limit?: number; offset?: number },
	opts?: { fetch?: Fetcher }
): Promise<DeviceSkillOccurrenceResponse> {
	const queryString = buildQueryString(page ?? {});
	return (await doGet(
		`/devices/skills/${encodeURIComponent(name)}/occurrences${queryString ? `?${queryString}` : ''}`,
		opts
	)) as DeviceSkillOccurrenceResponse;
}

export async function restartNanobotAgentDeployments(opts?: {
	fetch?: Fetcher;
	dryRun?: boolean;
}): Promise<RestartNanobotAgentDeploymentsResult> {
	const params = new URLSearchParams();
	if (opts?.dryRun != null) {
		params.set('dryRun', String(opts.dryRun));
	}
	const qs = params.toString();
	const path = qs
		? `/system-mcp-servers/restart-nanobot-agent-deployments?${qs}`
		: '/system-mcp-servers/restart-nanobot-agent-deployments';
	return (await doPost(path, {}, opts)) as RestartNanobotAgentDeploymentsResult;
}

export async function registerMcpServerOAuthDebuggerClient(
	serverID: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<OAuthDebuggerRegisterClientResponse> {
	return (await doPost(
		`/mcp-servers/${serverID}/oauth-debugger/client`,
		{},
		opts
	)) as OAuthDebuggerRegisterClientResponse;
}

export async function getMCPServerOAuthDebuggerAuthorizationURL(
	serverID: string,
	body: OAuthDebuggerAuthorizationURLRequest,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<OAuthDebuggerAuthorizationURL> {
	return (await doPost(
		`/mcp-servers/${serverID}/oauth-debugger/authorization-url`,
		body,
		opts
	)) as OAuthDebuggerAuthorizationURL;
}

export async function exchangeMCPServerOAuthDebuggerToken(
	serverID: string,
	body: OAuthDebuggerTokenRequest,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
): Promise<OAuthToken> {
	return (await doPost(`/mcp-servers/${serverID}/oauth-debugger/token`, body, opts)) as OAuthToken;
}
