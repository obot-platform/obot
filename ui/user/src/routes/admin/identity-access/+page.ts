import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { AuthProvider, GroupRoleAssignment, OrgGroup, OrgUser } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let users: OrgUser[] = [];
	let groups: OrgGroup[] = [];
	let groupRoleAssignments: GroupRoleAssignment[] = [];
	let defaultUsersRole: number | undefined;
	let authProviders: AuthProvider[] = [];
	let authEnabled = false;

	try {
		users = await UserService.listUsers({ fetch });
	} catch (err) {
		handleRouteError(err, `/users`, profile.current);
	}

	try {
		groupRoleAssignments = await AdminService.listGroupRoleAssignments({ fetch });
		try {
			groups = await UserService.resolveGroups(
				groupRoleAssignments.map((assignment) => assignment.groupName),
				{ fetch }
			);
		} catch (err) {
			console.error('Failed to resolve group names:', err);
		}
	} catch (err) {
		handleRouteError(err, `/admin/groups`, profile.current);
	}

	try {
		defaultUsersRole = await AdminService.getDefaultUsersRoleSettings({ fetch });
	} catch (err) {
		handleRouteError(err, `/user-configuration`, profile.current);
	}

	try {
		const version = await UserService.getVersion({ fetch });
		authEnabled = Boolean(version.authEnabled);
		if (authEnabled) {
			authProviders = await AdminService.listAuthProviders({ fetch });
		}
	} catch (err) {
		handleRouteError(err, '/v2/identity-access', profile.current);
	}

	return {
		users,
		groups,
		groupRoleAssignments,
		defaultUsersRole,
		authProviders,
		authEnabled
	};
};
