import type { DeviceClientSortKey } from '$lib/services/admin/types';
import type { DeviceSkillSortKey } from '$lib/services/admin/types';

// DEFAULT_WINDOW_MS is the rolling time range applied to the device
// overview when the URL doesn't pin start/end. Mirrors the backend
// dashboardWindowDefault in pkg/api/handlers/devicescans.go.
export const DEFAULT_WINDOW_MS = 60 * 24 * 60 * 60 * 1000;

// device clients

export const defaultClientSort = {
	property: 'name',
	order: 'asc'
} as const;

export const deviceClientSortFields: Record<string, DeviceClientSortKey> = {
	name: 'name',
	mcpServerCount: 'mcp_server_count',
	skillCount: 'skill_count',
	userCount: 'user_count'
};

export const DEFAULT_CLIENT_SORT_BY = deviceClientSortFields[defaultClientSort.property];
export const DEFAULT_CLIENT_SORT_ORDER = defaultClientSort.order;

// device skills

export const defaultSkillSort = {
	property: 'deviceCount',
	order: 'desc'
} as const;

export const deviceSkillSortFields: Record<string, DeviceSkillSortKey> = {
	name: 'name',
	deviceCount: 'device_count',
	userCount: 'user_count',
	observationCount: 'observation_count'
};

export const DEFAULT_SKILL_SORT_BY = deviceSkillSortFields[defaultSkillSort.property];
export const DEFAULT_SKILL_SORT_ORDER = defaultSkillSort.order;
