import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	type DeviceClientSortKey,
	type DeviceClientFleetSummaryResponse,
	type OrgUser,
	UserService
} from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const sortFields: Record<string, DeviceClientSortKey> = {
	name: 'name',
	mcpServerCount: 'mcp_server_count',
	skillCount: 'skill_count',
	userCount: 'user_count'
};
const DEFAULT_SORT_BY: DeviceClientSortKey = 'name';
const DEFAULT_SORT_ORDER = 'asc';

function getSortBy(property: string | null): DeviceClientSortKey {
	return sortFields[property ?? ''] ?? DEFAULT_SORT_BY;
}

function getSortOrder(order: string | null): 'asc' | 'desc' {
	return order === 'desc' ? 'desc' : 'asc';
}

export const load: PageLoad = async ({ fetch, url }) => {
	const limit = parseInt(url.searchParams.get('pageSize') ?? '50', 10) || 50;
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;
	const name = url.searchParams.get('name') ?? '';
	const sortProperty = url.searchParams.get('sort');
	const hasValidSortProperty = Boolean(sortFields[sortProperty ?? '']);
	const sortBy = getSortBy(sortProperty);
	const sortOrder = hasValidSortProperty
		? getSortOrder(url.searchParams.get('sortDirection'))
		: DEFAULT_SORT_ORDER;
	let clients: DeviceClientFleetSummaryResponse = {
		items: [],
		total: 0,
		limit,
		offset
	};
	let users: OrgUser[] = [];
	try {
		[clients, users] = await Promise.all([
			AdminService.listDeviceClients({ limit, offset, name, sortBy, sortOrder }, { fetch }),
			UserService.listUsers({ fetch })
		]);
		return { clients, users };
	} catch (err) {
		handleRouteError(err, '/admin/device-clients', profile.current);
	}
};
