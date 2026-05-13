import { AdminService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let hasDeviceScans = false;
	try {
		const response = await AdminService.listDeviceScans({ limit: 1 }, { fetch });
		hasDeviceScans = response.total > 0;
	} catch (_err) {
		hasDeviceScans = false;
	}

	return {
		hasDeviceScans
	};
};
