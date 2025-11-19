import {
	AdminService,
	ChatService,
	type MCPCatalogServer,
	type MCPServerInstance
} from '$lib/services';
import type { MCPCatalogEntry } from '$lib/services/admin/types';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('admin-mcp-server-and-entries');

export interface AdminMcpServerAndEntriesContext {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	userServerInstances: MCPServerInstance[];
	userConfiguredServers: MCPCatalogServer[];
	loading: boolean;
}

export function getAdminMcpServerAndEntries() {
	if (!hasContext(Key)) {
		throw new Error('Admin MCP server and entries not initialized');
	}
	return getContext<AdminMcpServerAndEntriesContext>(Key);
}

export function initMcpServerAndEntries(mcpServerAndEntries?: AdminMcpServerAndEntriesContext) {
	const data = $state<AdminMcpServerAndEntriesContext>(
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

export async function fetchMcpServerAndEntries(
	catalogId: string,
	mcpServerAndEntries?: AdminMcpServerAndEntriesContext,
	onSuccess?: (context: AdminMcpServerAndEntriesContext) => void
) {
	const context = mcpServerAndEntries || getAdminMcpServerAndEntries();
	context.loading = true;
	const [
		adminEntries,
		adminServers,
		workspaceEntries,
		workspaceServers,
		userConfiguredServers,
		userServerInstances
	] = await Promise.all([
		AdminService.listMCPCatalogEntries(catalogId, { all: true }),
		AdminService.listMCPCatalogServers(catalogId, { all: true }),
		AdminService.listAllUserWorkspaceCatalogEntries(),
		AdminService.listAllUserWorkspaceMCPServers(),
		ChatService.listSingleOrRemoteMcpServers(),
		ChatService.listMcpServerInstances()
	]);
	const entries = [...adminEntries, ...workspaceEntries];
	const servers = [...adminServers, ...workspaceServers];
	context.entries = entries;
	context.servers = servers;
	context.userConfiguredServers = userConfiguredServers;
	context.userServerInstances = userServerInstances;
	context.loading = false;

	if (onSuccess) {
		onSuccess(context);
	}
}
