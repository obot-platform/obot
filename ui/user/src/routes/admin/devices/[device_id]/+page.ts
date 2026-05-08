import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanResponse } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ params, fetch }) => {
	const { device_id } = params;

	let scans: DeviceScanResponse = { items: [], total: 0, limit: PAGE_SIZE, offset: 0 };
	try {
		scans = await AdminService.listDeviceScans(
			{
				limit: PAGE_SIZE,
				deviceId: [device_id],
				groupByDevice: false
			},
			{ fetch }
		);
		return { scans, deviceId: device_id, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, `/admin/devices/${device_id}`, profile.current);
	}
};
