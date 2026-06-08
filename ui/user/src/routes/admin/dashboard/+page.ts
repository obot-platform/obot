import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hasDeviceScans = false;
	try {
		const response = await UserService.listDeviceScans({ limit: 1 }, { fetch });
		hasDeviceScans = response.total > 0;
	} catch (err) {
		handleRouteError(err, '/admin/dashboard', profile);
	}

	return {
		hasDeviceScans
	};
};
