import { handleRouteError } from '$lib/errors';
import { UserService, type DeviceScanResponse } from '$lib/services';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	const { device_id } = params;

	let scans: DeviceScanResponse;
	try {
		scans = await UserService.listDeviceScans(
			{
				limit: PAGE_SIZE,
				deviceId: [device_id],
				groupByDevice: false
			},
			{ fetch }
		);
		return { scans, deviceId: device_id, pageSize: PAGE_SIZE };
	} catch (err) {
		const prefix = profile.hasAdminAccess?.() ? '/admin' : '';
		handleRouteError(err, `${prefix}/devices/${device_id}`, profile);
	}
};
