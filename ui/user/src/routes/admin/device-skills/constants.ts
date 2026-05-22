import type { DeviceSkillSortKey } from '$lib/services/admin/types';

export const PAGE_SIZE = 50;

export const defaultSort = {
	property: 'deviceCount',
	order: 'desc'
} as const;

export const sortFields: Record<string, DeviceSkillSortKey> = {
	name: 'name',
	deviceCount: 'device_count',
	userCount: 'user_count',
	observationCount: 'observation_count'
};

export const DEFAULT_SORT_BY = sortFields[defaultSort.property];
export const DEFAULT_SORT_ORDER = defaultSort.order;
