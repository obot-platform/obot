import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let defaultUsersRole: number | undefined;
	try {
		defaultUsersRole = await AdminService.getDefaultUsersRoleSettings({ fetch });
	} catch (err) {
		handleRouteError(err, `/user-configuration`, profile.current);
	}

	return {
		defaultUsersRole
	};
};
