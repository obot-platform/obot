import { handleRouteError } from '$lib/errors';
import { AdminService, UserService, type OrgGroup } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const groupRoleAssignments = await AdminService.listGroupRoleAssignments({ fetch });

		// Resolve names only for the groups that actually have an assignment. Listing the whole
		// directory would both be needlessly large and drop any assignment whose group fell outside
		// the page that came back.
		let groups: OrgGroup[] = [];
		try {
			groups = await UserService.resolveGroups(
				groupRoleAssignments.map((assignment) => assignment.groupName),
				{ fetch }
			);
		} catch (err) {
			console.error('Failed to resolve group names:', err);
		}

		return { groups, groupRoleAssignments };
	} catch (err) {
		handleRouteError(err, `/admin/groups`, profile.current);

		return {
			groups: [],
			groupRoleAssignments: []
		};
	}
};
