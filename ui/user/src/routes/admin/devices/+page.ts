import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanList } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ url, fetch }) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;

	let devices: DeviceScanList = { items: [], total: 0, limit: PAGE_SIZE, offset };
	try {
		devices = await AdminService.listDeviceScans(
			{ limit: PAGE_SIZE, offset, groupByDevice: true },
			{ fetch }
		);
		return { devices, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, '/admin/devices', profile.current);
	}
};
