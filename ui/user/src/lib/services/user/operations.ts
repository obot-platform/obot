import { BOOTSTRAP_USER_ID } from '$lib/constants';
import { Group } from '$lib/services/admin/types';
import type {
	AccessControlRule,
	AccessControlRuleManifest,
	AuthProvider,
	K8sServerDetail,
	MCPCatalogEntry,
	MCPCatalogEntryServerManifest,
	MCPCatalogServerManifest,
	ServerK8sSettings,
	MCPServerOAuthCredentialRequest,
	MCPServerOAuthCredentialStatus
} from '../admin/types';
import { doDelete, doGet, doPatch, doPost, doPut, type Fetcher } from '../http';
import {
	type DefaultModelAlias,
	type ImageResponse,
	type MCPCatalogServer,
	type MCPServerInstance,
	type MCPServerPrompt,
	type McpServerResource,
	type MCPServerTool,
	type Model,
	type ModelProviderList,
	type Profile,
	type Version,
	type Workspace
} from './types';

type ItemsResponse<T> = { items: T[] | null };

export async function getProfile(opts?: { fetch?: Fetcher }): Promise<Profile> {
	const obj = (await doGet('/me', opts)) as Profile;
	obj.isAdmin = () => {
		return obj.groups.includes(Group.ADMIN);
	};
	obj.hasAdminAccess = () => {
		return obj.groups.includes(Group.ADMIN) || obj.groups.includes(Group.AUDITOR);
	};
	obj.isAdminReadonly = () => {
		return !obj.groups.includes(Group.ADMIN) && obj.groups.includes(Group.AUDITOR);
	};
	obj.isBootstrapUser = () => {
		return obj.username === BOOTSTRAP_USER_ID;
	};
	obj.canImpersonate = () => {
		return obj.groups.includes(Group.USER_IMPERSONATION) && obj.groups.includes(Group.ADMIN);
	};
	obj.loaded = true;
	return obj;
}

export async function deleteProfile() {
	return doDelete(`/me`);
}

export async function patchProfile(
	profile: Partial<Profile>,
	opts?: { dontLogErrors?: boolean }
): Promise<Profile> {
	return (await doPatch('/me', profile, opts)) as Profile;
}

export async function getVersion(opts?: { fetch?: Fetcher }): Promise<Version> {
	const version = (await doGet('/version', opts)) as Version;
	return version;
}

export async function listAuthProviders(opts?: { fetch?: Fetcher }): Promise<AuthProvider[]> {
	const list = (await doGet('/auth-providers', opts)) as ItemsResponse<AuthProvider>;
	if (!list.items) {
		list.items = [];
	}
	return list.items.filter((provider) => provider.configured);
}

export async function listMCPs(opts?: { fetch?: Fetcher }): Promise<MCPCatalogEntry[]> {
	const response = (await doGet('/all-mcps/entries', opts)) as ItemsResponse<MCPCatalogEntry>;
	return (
		response.items?.map((item) => {
			return {
				...item,
				isCatalogEntry: true
			};
		}) ?? []
	);
}

export async function getMCP(id: string, opts?: { fetch?: Fetcher }): Promise<MCPCatalogEntry> {
	const response = (await doGet(`/all-mcps/entries/${id}`, opts)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function listMCPCatalogServers(opts?: {
	fetch?: Fetcher;
}): Promise<MCPCatalogServer[]> {
	const response = (await doGet('/all-mcps/servers', opts)) as {
		items: MCPCatalogServer[] | null;
	};
	return response.items ?? [];
}

export async function getMcpCatalogServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	return (await doGet(`/all-mcps/servers/${id}`, opts)) as MCPCatalogServer;
}

export async function listMcpCatalogServerTools(
	id: string,
	opts?: { fetch?: Fetcher; signal?: AbortSignal }
): Promise<MCPServerTool[]> {
	try {
		return (await doGet(`/all-mcps/servers/${id}/tools`, {
			...opts,
			dontLogErrors: true
		})) as MCPServerTool[];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
}

export async function listMcpCatalogServerPrompts(
	id: string,
	opts?: { fetch?: Fetcher; signal?: AbortSignal }
): Promise<MCPServerPrompt[]> {
	try {
		return (await doGet(`/all-mcps/servers/${id}/prompts`, {
			...opts,
			dontLogErrors: true
		})) as MCPServerPrompt[];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
}

export async function listMcpCatalogServerResources(
	id: string,
	opts?: { fetch?: Fetcher; signal?: AbortSignal }
): Promise<McpServerResource[]> {
	try {
		return (await doGet(`/all-mcps/servers/${id}/resources`, {
			...opts,
			dontLogErrors: true
		})) as McpServerResource[];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
}

export async function listModels(opts?: { fetch?: Fetcher }): Promise<Model[]> {
	const response = (await doGet('/models', opts)) as ItemsResponse<Model>;
	return response.items ?? [];
}

export async function listGlobalModelProviders(opts?: {
	fetch?: Fetcher;
}): Promise<ModelProviderList> {
	const response = (await doGet('/model-providers', opts)) as ModelProviderList;
	return response;
}

export async function listSingleOrRemoteMcpServers(opts?: {
	fetch?: Fetcher;
}): Promise<MCPCatalogServer[]> {
	const response = (await doGet('/mcp-servers', opts)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function getSingleOrRemoteMcpServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doGet(`/mcp-servers/${id}`, opts)) as MCPCatalogServer;
	return response;
}

export async function createSingleOrRemoteMcpServer(server: {
	catalogEntryID?: string;
	manifest?: {
		remoteConfig?: {
			url?: string;
		};
	};
	alias?: string;
}): Promise<MCPCatalogServer> {
	const response = (await doPost('/mcp-servers', server)) as MCPCatalogServer;
	return response;
}

export async function createCompositeMcpServer(server: {
	catalogEntryID?: string;
	manifest?: {
		compositeConfig?: {
			componentServers: Array<{
				catalogEntryID?: string;
				mcpServerID?: string;
				manifest?: {
					remoteConfig?: {
						url?: string;
					};
				};
				disabled?: boolean;
			}>;
		};
	};
	alias?: string;
}): Promise<MCPCatalogServer> {
	const response = (await doPost('/mcp-servers', server)) as MCPCatalogServer;
	return response;
}

export async function updateRemoteMcpServerUrl(id: string, url: string): Promise<void> {
	await doPost(`/mcp-servers/${id}/update-url`, { url });
}

export async function updateSingleOrRemoteMcpServerAlias(id: string, alias: string): Promise<void> {
	await doPut(`/mcp-servers/${id}/alias`, { alias });
}

export async function deleteSingleOrRemoteMcpServer(id: string): Promise<void> {
	await doDelete(`/mcp-servers/${id}`);
}

export async function clearMcpServerOAuth(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/mcp-servers/${id}/oauth`, opts);
}

export async function configureSingleOrRemoteMcpServer(
	id: string,
	envs: Record<string, string>
): Promise<MCPCatalogServer> {
	const response = (await doPost(`/mcp-servers/${id}/configure`, envs)) as MCPCatalogServer;
	return response;
}

export async function configureCompositeMcpServer(
	id: string,
	componentConfigs: Record<
		string,
		{ config: Record<string, string>; url?: string; disabled?: boolean }
	>
): Promise<MCPCatalogServer> {
	const response = (await doPost(`/mcp-servers/${id}/configure`, {
		componentConfigs
	})) as MCPCatalogServer;
	return response;
}

export async function deconfigureSingleOrRemoteMcpServer(id: string): Promise<void> {
	await doPost(`/mcp-servers/${id}/deconfigure`, {});
}

export async function deconfigureCompositeMcpServer(id: string): Promise<void> {
	return deconfigureSingleOrRemoteMcpServer(id);
}

export async function restartMcpServer(id: string, opts?: { fetch?: Fetcher }): Promise<void> {
	await doPost(`/mcp-servers/${id}/restart`, {}, opts);
}

// Update any MCP server manifest (used for composite skips)
export async function updateMcpServerManifest(
	id: string,
	manifest: MCPCatalogServerManifest
): Promise<MCPCatalogServer> {
	const response = (await doPut(`/mcp-servers/${id}`, manifest)) as MCPCatalogServer;
	return response;
}

export async function revealSingleOrRemoteMcpServer(
	id: string,
	opts?: { dontLogErrors?: boolean }
): Promise<Record<string, string>> {
	return doPost(`/mcp-servers/${id}/reveal`, {}, opts) as Promise<Record<string, string>>;
}

export async function revealCompositeMcpServer(
	id: string,
	opts?: { dontLogErrors?: boolean }
): Promise<{
	componentConfigs: Record<
		string,
		{ config: Record<string, string>; url?: string; disabled?: boolean }
	>;
}> {
	return doPost(`/mcp-servers/${id}/reveal`, {}, opts) as Promise<{
		componentConfigs: Record<
			string,
			{ config: Record<string, string>; url?: string; disabled?: boolean }
		>;
	}>;
}

export async function listSingleOrRemoteMcpServerTools(id: string): Promise<MCPServerTool[]> {
	try {
		const response = (await doGet(`/mcp-servers/${id}/tools`, {
			dontLogErrors: true
		})) as ItemsResponse<MCPServerTool>;
		return response.items ?? [];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
}

export async function listSingleOrRemoteMcpServerPrompts(id: string): Promise<MCPServerPrompt[]> {
	try {
		const response = (await doGet(`/mcp-servers/${id}/prompts`, {
			dontLogErrors: true
		})) as ItemsResponse<MCPServerPrompt>;
		return response.items ?? [];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
}

export async function listSingleOrRemoteMcpServerResources(
	id: string
): Promise<McpServerResource[]> {
	try {
		const response = (await doGet(`/mcp-servers/${id}/resources`, {
			dontLogErrors: true
		})) as ItemsResponse<McpServerResource>;
		return response.items ?? [];
	} catch (error) {
		if (error instanceof Error && error.message.startsWith('424')) {
			return [];
		}
		throw error;
	}
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

export async function createMcpServerInstance(mcpServerID: string): Promise<MCPServerInstance> {
	const response = (await doPost('/mcp-server-instances', {
		mcpServerID
	})) as MCPServerInstance;
	return response;
}

export async function configureMcpServerInstance(
	id: string,
	envs: Record<string, string>
): Promise<MCPServerInstance> {
	const response = (await doPost(
		`/mcp-server-instances/${id}/configure`,
		envs
	)) as MCPServerInstance;
	return response;
}

export async function revealMcpServerInstance(
	id: string,
	opts?: { dontLogErrors?: boolean }
): Promise<Record<string, string>> {
	const response = (await doPost(`/mcp-server-instances/${id}/reveal`, {}, opts)) as Record<
		string,
		string
	> | null;
	return response ?? {};
}

export async function deleteMcpServerInstance(id: string): Promise<void> {
	await doDelete(`/mcp-server-instances/${id}`);
}

// 412 means oauth is needed
export async function getMcpServerOauthURL(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<string> {
	try {
		const response = (await doGet(`/mcp-servers/${id}/oauth-url`, {
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

export async function isMcpServerOauthNeeded(
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<boolean> {
	try {
		await doPost(`/mcp-servers/${id}/check-oauth`, {
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

export async function triggerMcpServerUpdate(mcpServerId: string): Promise<MCPCatalogServer> {
	return (await doPost(`/mcp-servers/${mcpServerId}/trigger-update`, {})) as MCPCatalogServer;
}

export async function triggerWorkspaceMcpServerUpdate(
	workspaceID: string,
	entryID: string,
	mcpServerId: string
): Promise<MCPCatalogServer> {
	return (await doPost(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}/trigger-update`,
		{}
	)) as MCPCatalogServer;
}

export async function validateSingleOrRemoteMcpServerLaunched(mcpServerId: string): Promise<{
	success: boolean;
	message?: string;
	code?: number;
}> {
	try {
		await doPost(`/mcp-servers/${mcpServerId}/launch`, {}, { dontLogErrors: true });
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

export async function listSingleOrRemoteMcpServerLogs(mcpServerId: string): Promise<string[]> {
	const response = (await doGet(`/mcp-servers/${mcpServerId}/logs`, {
		dontLogErrors: true
	})) as ItemsResponse<string>;
	return response.items ?? [];
}

export async function listWorkspaces(opts?: { fetch?: Fetcher }): Promise<Workspace[]> {
	const response = (await doGet('/workspaces', opts)) as ItemsResponse<Workspace>;
	return response.items ?? [];
}

export async function listWorkspaceMCPCatalogEntries(
	workspaceID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry[]> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries`,
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

export async function getWorkspaceMCPCatalogEntry(
	workspaceID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}`,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function listWorkspaceMCPCatalogServers(
	workspaceID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer[]> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function getWorkspaceMCPCatalogServer(
	workspaceID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/servers/${serverID}`,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function generateWorkspaceMCPCatalogEntryToolPreviews(
	workspaceID: string,
	entryID: string,
	body?: {
		config?: Record<string, string>;
		url?: string;
	},
	opts?: { fetch?: Fetcher }
): Promise<void> {
	await doPost(`/workspaces/${workspaceID}/entries/${entryID}/generate-tool-previews`, body ?? {}, {
		...opts,
		dontLogErrors: true
	});
}

export async function getWorkspaceMCPCatalogEntryToolPreviewsOauth(
	workspaceID: string,
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
	opts?: { fetch?: Fetcher }
): Promise<string | Record<string, string>> {
	try {
		const response = (await doPost(
			`/workspaces/${workspaceID}/entries/${entryID}/generate-tool-previews/oauth-url`,
			body ?? {},
			{
				...opts,
				dontLogErrors: true
			}
		)) as
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

export async function deleteWorkspaceMCPCatalogServer(
	workspaceID: string,
	serverID: string
): Promise<void> {
	await doDelete(`/workspaces/${workspaceID}/servers/${serverID}`);
}

export async function deleteWorkspaceMCPCatalogEntry(
	workspaceID: string,
	entryID: string
): Promise<void> {
	await doDelete(`/workspaces/${workspaceID}/entries/${entryID}`);
}

export async function listWorkspaceMCPServersForEntry(
	workspaceID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer[]> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}/servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function listWorkspaceMcpCatalogServerInstances(
	workspaceID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/servers/${mcpServerId}/instances`,
		opts
	)) as ItemsResponse<MCPServerInstance>;
	return response.items ?? [];
}

export async function revealWorkspaceMCPCatalogServer(
	workspaceID: string,
	serverID: string,
	opts?: { fetch?: Fetcher }
): Promise<Record<string, string>> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/servers/${serverID}/reveal`,
		{},
		{
			...opts,
			dontLogErrors: true
		}
	)) as Record<string, string>;
	return response;
}

export async function updateWorkspaceMCPCatalogEntry(
	workspaceID: string,
	entryID: string,
	entry: MCPCatalogEntryServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPut(
		`/workspaces/${workspaceID}/entries/${entryID}`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function createWorkspaceMCPCatalogEntry(
	workspaceID: string,
	entry: MCPCatalogEntryServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogEntry> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/entries`,
		entry,
		opts
	)) as MCPCatalogEntry;
	return {
		...response,
		isCatalogEntry: true
	};
}

export async function updateWorkspaceMCPCatalogServer(
	workspaceID: string,
	serverID: string,
	server: MCPCatalogServerManifest['manifest'],
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPut(
		`/workspaces/${workspaceID}/servers/${serverID}`,
		server,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function createWorkspaceMCPCatalogServer(
	workspaceID: string,
	server: MCPCatalogServerManifest,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/servers`,
		server,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function configureWorkspaceMCPCatalogServer(
	workspaceID: string,
	serverID: string,
	envs: Record<string, string>,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/servers/${serverID}/configure`,
		envs,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function listWorkspaceAccessControlRules(
	workspaceID: string,
	opts?: {
		fetch?: Fetcher;
	}
): Promise<AccessControlRule[]> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/access-control-rules`,
		opts
	)) as ItemsResponse<AccessControlRule>;
	return response.items ?? [];
}

export async function getWorkspaceAccessControlRule(
	workspaceID: string,
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<AccessControlRule> {
	const response = (await doGet(
		`/workspaces/${workspaceID}/access-control-rules/${id}`,
		opts
	)) as AccessControlRule;
	return response;
}

export async function createWorkspaceAccessControlRule(
	workspaceID: string,
	rule: AccessControlRuleManifest
): Promise<AccessControlRule> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/access-control-rules`,
		rule
	)) as AccessControlRule;
	return response;
}

export async function updateWorkspaceAccessControlRule(
	workspaceID: string,
	id: string,
	rule: AccessControlRuleManifest
): Promise<AccessControlRule> {
	return (await doPut(
		`/workspaces/${workspaceID}/access-control-rules/${id}`,
		rule
	)) as AccessControlRule;
}

export async function deleteWorkspaceAccessControlRule(
	workspaceID: string,
	id: string
): Promise<void> {
	await doDelete(`/workspaces/${workspaceID}/access-control-rules/${id}`);
}

export async function fetchWorkspaceIDForProfile(
	profileID?: string,
	opts?: { fetch?: Fetcher }
): Promise<string> {
	const currentProfileID = profileID ? profileID : (await getProfile(opts)).id;
	const workspaces = await listWorkspaces(opts);
	const workspaceID = workspaces.find((w) => w.userID === currentProfileID)?.id ?? null;
	if (!workspaceID) {
		throw new Error('404 Workspace not found.');
	}
	return workspaceID;
}

// 412 means oauth is needed
export async function getWorkspaceMcpServerOauthURL(
	workspaceID: string,
	id: string,
	opts?: { signal?: AbortSignal }
): Promise<string> {
	try {
		const response = (await doGet(`/workspaces/${workspaceID}/servers/${id}/oauth-url`, {
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

export async function getWorkspaceK8sServerDetail(
	workspaceID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/servers/${mcpServerId}/details`,
		opts
	)) as K8sServerDetail;
	return response;
}

export async function restartWorkspaceK8sServerDeployment(
	workspaceID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	await doPost(`/workspaces/${workspaceID}/servers/${mcpServerId}/restart`, {}, opts);
}

export async function getWorkspaceK8sServerStatus(
	workspaceID: string,
	mcpServerId: string,
	opts?: { dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/servers/${mcpServerId}/k8s-settings-status`,
		opts
	)) as ServerK8sSettings;
	return response;
}

export async function redeployWorkspaceK8sServerWithK8sSettings(
	workspaceID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost(
		`/workspaces/${workspaceID}/servers/${mcpServerId}/redeploy-with-k8s-settings`,
		{},
		opts
	);
	return response;
}

export async function getWorkspaceCatalogEntryServers(
	workspaceID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}/servers`,
		opts
	)) as ItemsResponse<MCPCatalogServer>;
	return response.items ?? [];
}

export async function getWorkspaceCatalogEntryServer(
	workspaceID: string,
	entryID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}`,
		opts
	)) as MCPCatalogServer;
	return response;
}

export async function getWorkspaceCatalogEntryServerK8sDetails(
	workspaceID: string,
	entryID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher; dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}/details`,
		opts
	)) as K8sServerDetail;
	return response;
}

// Composite MCP OAuth helpers
export type PendingCompositeAuth = {
	catalogEntryID?: string;
	mcpServerID: string;
	authURL: string;
};

export async function checkCompositeOAuth(
	compositeMcpId: string,
	opts?: { oauthAuthRequestID?: string; signal?: AbortSignal }
): Promise<PendingCompositeAuth[]> {
	let url = `/oauth/composite/${compositeMcpId}`;
	if (opts?.oauthAuthRequestID) {
		url += `?oauth_auth_request=${opts.oauthAuthRequestID}`;
	}
	const response = await doGet(url, { signal: opts?.signal, dontLogErrors: true });

	// If the server returns a redirect_uri, perform client-side redirect
	if (response && typeof response === 'object' && 'redirect_uri' in response) {
		window.location.href = (response as { redirect_uri: string }).redirect_uri;
		return [];
	}

	return Array.isArray(response) ? response : [];
}

export async function restartWorkspaceCatalogEntryServerDeployment(
	workspaceID: string,
	entryID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	await doPost(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}/restart`,
		{},
		opts
	);
}

export async function getWorkspaceCatalogEntryServerK8sSettingsStatus(
	workspaceID: string,
	entryID: string,
	mcpServerId: string,
	opts?: { dontLogErrors?: boolean }
) {
	const response = (await doGet(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}/k8s-settings-status`,
		opts
	)) as ServerK8sSettings;
	return response;
}

export async function redeployWorkspaceCatalogEntryServerWithK8sSettings(
	workspaceID: string,
	entryID: string,
	mcpServerId: string,
	opts?: { fetch?: Fetcher }
) {
	const response = await doPost(
		`/workspaces/${workspaceID}/entries/${entryID}/servers/${mcpServerId}/redeploy-with-k8s-settings`,
		{},
		opts
	);
	return response;
}

// GET /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
export async function getWorkspaceMCPCatalogEntryOAuthCredentials(
	workspaceID: string,
	entryID: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerOAuthCredentialStatus> {
	const response = (await doGet(`/workspaces/${workspaceID}/entries/${entryID}/oauth-credentials`, {
		...opts,
		dontLogErrors: true
	})) as MCPServerOAuthCredentialStatus;
	return response;
}

// POST /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
export async function setWorkspaceMCPCatalogEntryOAuthCredentials(
	workspaceID: string,
	entryID: string,
	credentials: MCPServerOAuthCredentialRequest,
	opts?: { fetch?: Fetcher }
): Promise<MCPServerOAuthCredentialStatus> {
	const response = (await doPost(
		`/workspaces/${workspaceID}/entries/${entryID}/oauth-credentials`,
		credentials,
		opts
	)) as MCPServerOAuthCredentialStatus;
	return response;
}

// DELETE /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
export async function deleteWorkspaceMCPCatalogEntryOAuthCredentials(
	workspaceID: string,
	entryID: string,
	opts?: { signal?: AbortSignal }
): Promise<void> {
	await doDelete(`/workspaces/${workspaceID}/entries/${entryID}/oauth-credentials`, opts);
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

export async function uploadImage(file: File): Promise<ImageResponse> {
	const formData = new FormData();
	formData.append('image', file);

	return (await doPost('/image/upload', formData)) as ImageResponse;
}
