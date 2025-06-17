import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let users: OrgUser[] = [];
	try {
		users = await AdminService.listUsers({ fetch });
	} catch (err) {
		handleRouteError(err, `/users`, profile.current);
	}

	return {
		users
	};
};
