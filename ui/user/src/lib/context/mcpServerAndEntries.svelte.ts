import { ChatService, type MCPCatalogServer, type MCPServerInstance } from '$lib/services';
import type { MCPCatalogEntry } from '$lib/services/admin/types';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('user-mcp-server-and-entries');

export interface UserMcpServerAndEntriesContext {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	userServerInstances: MCPServerInstance[];
	userConfiguredServers: MCPCatalogServer[];
	loading: boolean;
}

export function getUserMcpServerAndEntries() {
	if (!hasContext(Key)) {
		throw new Error('User MCP server and entries not initialized');
	}
	return getContext<UserMcpServerAndEntriesContext>(Key);
}

export function initUserMcpServerAndEntries(mcpServerAndEntries?: UserMcpServerAndEntriesContext) {
	const data = $state<UserMcpServerAndEntriesContext>(
		mcpServerAndEntries ?? {
			entries: [],
			servers: [],
			userServerInstances: [],
			userConfiguredServers: [],
			loading: false
		}
	);
	setContext(Key, data);
}

export async function fetchUserMcpServerAndEntries(
	mcpServerAndEntries?: UserMcpServerAndEntriesContext,
	onSuccess?: (context: UserMcpServerAndEntriesContext) => void
) {
	const context = mcpServerAndEntries || getUserMcpServerAndEntries();
	context.loading = true;
	const [userConfiguredServers, entriesResult, serversResult, userServerInstances] =
		await Promise.all([
			ChatService.listSingleOrRemoteMcpServers(),
			ChatService.listMCPs(),
			ChatService.listMCPCatalogServers(),
			ChatService.listMcpServerInstances()
		]);

	context.userConfiguredServers = userConfiguredServers;
	context.entries = entriesResult;
	context.servers = serversResult;
	context.userServerInstances = userServerInstances;
	context.loading = false;

	if (onSuccess) {
		onSuccess(context);
	}
}
