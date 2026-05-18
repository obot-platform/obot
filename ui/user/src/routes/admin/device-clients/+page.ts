import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	type DeviceClientFleetSummaryResponse,
	type OrgUser,
	UserService
} from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const limit = parseInt(url.searchParams.get('pageSize') ?? '50', 10) || 50;
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;
	const name = url.searchParams.get('name') ?? '';
	let clients: DeviceClientFleetSummaryResponse = {
		items: [],
		total: 0,
		limit,
		offset
	};
	let users: OrgUser[] = [];
	try {
		[clients, users] = await Promise.all([
			AdminService.listDeviceClients({ limit, offset, name }, { fetch }),
			UserService.listUsers({ fetch })
		]);
		return { clients, users };
	} catch (err) {
		handleRouteError(err, '/admin/device-clients', profile.current);
	}
};
