import { handleRouteError } from '$lib/errors';
import { UserService, type OrgUser, type DeviceScanResponse } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ url, fetch }) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;

	let devices: DeviceScanResponse = { items: [], total: 0, limit: PAGE_SIZE, offset };
	let users: OrgUser[] = [];
	try {
		[devices, users] = await Promise.all([
			UserService.listDeviceScans({ limit: PAGE_SIZE, offset, groupByDevice: true }, { fetch }),
			UserService.listUsers({ fetch }).catch(() => [] as OrgUser[])
		]);
		return { devices, users, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, '/admin/devices', profile.current);
	}
};
