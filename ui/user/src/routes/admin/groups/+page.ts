import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const allGroupsPromise = await AdminService.listGroups({ fetch, includeRestricted: true });

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
