import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const allGroupsPromise = await UserService.listGroups({ fetch });

	try {
		return {
			groups: await allGroupsPromise,
			groupRoleAssignments: await AdminService.listGroupRoleAssignments({ fetch })
		};
	} catch (err) {
		handleRouteError(err, `/admin/groups`, profile.current);

		return {
			groups: await Promise.resolve([]),
			groupRoleAssignments: await Promise.resolve([])
		};
	}
};
