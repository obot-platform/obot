import { handleRouteError } from '$lib/errors';
import { UserService, type OrgUser } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let users: OrgUser[] = [];
	try {
		users = await UserService.listUsers({ fetch });
	} catch (err) {
		handleRouteError(err, `/users`, profile.current);
	}

	return {
		users
	};
};
