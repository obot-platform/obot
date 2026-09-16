import { handleRouteError } from '$lib/errors';
import { Group, UserService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	const isAtLeastPoweruser =
		profile?.groups.includes(Group.POWERUSER) || profile?.hasAdminAccess?.();
	if (!isAtLeastPoweruser) {
		throw redirect(302, '/vmcps');
	}

	let hasDeviceScans = false;
	try {
		const response = await UserService.listDeviceScans({ limit: 1 }, { fetch });
		hasDeviceScans = response.total > 0;
	} catch (err) {
		handleRouteError(err, '/dashboard', profile);
	}

	return {
		hasDeviceScans
	};
};
