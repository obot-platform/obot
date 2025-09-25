import { ChatService, Group, type MCPCatalogServer } from '$lib/services';
import type { MCPCatalogEntry } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('poweruser-workspace');

export interface PoweruserWorkspaceContext {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	loading: boolean;
}

export function getPoweruserWorkspace() {
	if (!hasContext(Key)) {
		throw new Error('Workspace MCP server and entries not initialized');
	}
	return getContext<PoweruserWorkspaceContext>(Key);
}

export function initMcpServerAndEntries(mcpServerAndEntries?: PoweruserWorkspaceContext) {
	const data = $state<PoweruserWorkspaceContext>(
		mcpServerAndEntries ?? {
			entries: [],
			servers: [],
			loading: false
		}
	);
	setContext(Key, data);
}

export async function fetchMcpServerAndEntries(
	workspaceID: string,
	mcpServerAndEntries?: PoweruserWorkspaceContext,
	onSuccess?: (entries: MCPCatalogEntry[], servers: MCPCatalogServer[]) => void
) {
	const context = mcpServerAndEntries || getPoweruserWorkspace();
	context.loading = true;
	const hasMultiUserAccess = profile.current.groups.includes(Group.POWERUSER_PLUS);
	const entries = await ChatService.listWorkspaceMCPCatalogEntries(workspaceID);
	// if not power user plus/admin, skip multi-users servers call
	const servers = hasMultiUserAccess
		? await ChatService.listWorkspaceMCPCatalogServers(workspaceID)
		: [];
	context.entries = entries;
	context.servers = servers;
	context.loading = false;

	if (onSuccess) {
		onSuccess(entries, servers);
	}
}
