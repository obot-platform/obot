import { AdminService, type MCPCatalogServer } from '$lib/services';
import type { MCPCatalogEntry } from '$lib/services/admin/types';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('admin-mcp-server-and-entries');

export interface AdminMcpServerAndEntriesContext {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
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
			loading: false
		}
	);
	setContext(Key, data);
}

export async function fetchMcpServerAndEntries(
	catalogId: string,
	mcpServerAndEntries?: AdminMcpServerAndEntriesContext,
	onSuccess?: (entries: MCPCatalogEntry[], servers: MCPCatalogServer[]) => void
) {
	const context = mcpServerAndEntries || getAdminMcpServerAndEntries();
	context.loading = true;
	const entries = await AdminService.listMCPCatalogEntries(catalogId);
	const servers = await AdminService.listMCPCatalogServers(catalogId);
	context.entries = entries;
	context.servers = servers;
	context.loading = false;

	if (onSuccess) {
		onSuccess(entries, servers);
	}
}
