import type { DeviceClientSortKey } from '$lib/services/admin/types';

// DEFAULT_WINDOW_MS is the rolling time range applied to the device
// overview when the URL doesn't pin start/end. Mirrors the backend
// dashboardWindowDefault in pkg/api/handlers/devicescans.go.
export const DEFAULT_WINDOW_MS = 60 * 24 * 60 * 60 * 1000;

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
