import type { DeviceClientSortKey } from '$lib/services/admin/types';

export const defaultSort = {
	property: 'name',
	order: 'asc'
} as const;

export const sortFields: Record<string, DeviceClientSortKey> = {
	name: 'name',
	mcpServerCount: 'mcp_server_count',
	skillCount: 'skill_count',
	userCount: 'user_count'
};

export const DEFAULT_SORT_BY = sortFields[defaultSort.property];
export const DEFAULT_SORT_ORDER = defaultSort.order;
