import { ChatService, type MCPCatalogServer } from '$lib/services';
import type { MCPCatalogEntry } from '$lib/services/admin/types';
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
	const entries = await ChatService.listWorkspaceMCPCatalogEntries(workspaceID);
	const servers = await ChatService.listMCPCatalogServers();
	context.entries = entries;
	context.servers = servers;
	context.loading = false;

	if (onSuccess) {
		onSuccess(entries, servers);
	}
}
