import { handleRouteError } from '$lib/errors';
import { UserService, type DeviceScanResponse } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let deviceScans: DeviceScanResponse = { items: [], total: 0, limit: 1, offset: 0 };
	try {
		deviceScans = await UserService.listDeviceScans({ limit: 1 }, { fetch });
	} catch (err) {
		handleRouteError(err, '/dashboard', profile);
	}

	return {
		deviceScans
	};
};
