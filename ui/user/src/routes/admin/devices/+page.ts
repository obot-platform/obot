import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanResponse, OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ url, fetch }) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;

	let devices: DeviceScanResponse = { items: [], total: 0, limit: PAGE_SIZE, offset };
	let users: OrgUser[] = [];
	try {
		[devices, users] = await Promise.all([
			AdminService.listDeviceScans({ limit: PAGE_SIZE, offset, groupByDevice: true }, { fetch }),
			AdminService.listUsers({ fetch }).catch(() => [] as OrgUser[])
		]);
		return { devices, users, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, '/admin/devices', profile.current);
	}
};
