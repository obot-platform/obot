import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ params, fetch, parent }) => {
	const { device_id, scan_id } = params;
	const { profile } = await parent();

	try {
		const scan = await UserService.getDeviceScan(scan_id, { fetch });
		return { scan };
	} catch (err) {
		const urlPrefix = profile.hasAdminAccess?.() ? '/admin' : '';
		handleRouteError(err, `${urlPrefix}/devices/${device_id}/scans/${scan_id}`, profile);
	}
};
