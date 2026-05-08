import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ params, fetch, parent }) => {
	const { device_id, scan_id } = params;
	const { profile } = await parent();

	try {
		const scan = await AdminService.getDeviceScan(scan_id, { fetch });
		return { scan };
	} catch (err) {
		handleRouteError(err, `/admin/devices/${device_id}/scans/${scan_id}`, profile);
	}
};
