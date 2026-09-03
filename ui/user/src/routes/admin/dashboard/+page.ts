import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	let hasDeviceScans = false;
	const [parentData, scansResult] = await Promise.all([
		parent(),
		UserService.listDeviceScans({ limit: 1 }, { fetch })
			.then((response) => ({ response }))
			.catch((error: unknown) => ({ error }))
	]);

	if ('response' in scansResult) {
		hasDeviceScans = scansResult.response.total > 0;
	} else {
		handleRouteError(scansResult.error, '/admin/dashboard', parentData.profile);
	}

	return {
		hasDeviceScans
	};
};
