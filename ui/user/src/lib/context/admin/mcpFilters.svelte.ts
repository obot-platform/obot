import { AdminService } from '$lib/services';
import type { MCPFilter } from '$lib/services/admin/types';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('admin-mcp-filters');

export interface AdminMcpFiltersContext {
	filters: MCPFilter[];
	loading: boolean;
}

export function getAdminMcpFilters() {
	if (!hasContext(Key)) {
		throw new Error('Admin MCP filters not initialized');
	}
	return getContext<AdminMcpFiltersContext>(Key);
}

export function initMcpFilters(mcpFilters?: AdminMcpFiltersContext) {
	const data = $state<AdminMcpFiltersContext>(
		mcpFilters ?? {
			filters: [],
			loading: false
		}
	);
	setContext(Key, data);
}

export async function fetchMcpFilters(
	mcpFilters?: AdminMcpFiltersContext,
	onSuccess?: (filters: MCPFilter[]) => void
) {
	const context = mcpFilters || getAdminMcpFilters();
	context.loading = true;
	const filters = await AdminService.listMCPFilters();
	context.filters = filters;
	context.loading = false;

	if (onSuccess) {
		onSuccess(filters);
	}
}
