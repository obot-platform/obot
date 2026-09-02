import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { AuthProvider, GroupRoleAssignment, OrgGroup, OrgUser } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const views = new Set(['users', 'groups', 'roles', 'auth-providers']);

export const load: PageLoad = async ({ fetch, url }) => {
	const requestedView = url.searchParams.get('view');
	const view = requestedView && views.has(requestedView) ? requestedView : 'users';

	let users: OrgUser[] = [];
	let groups: OrgGroup[] = [];
	let groupRoleAssignments: GroupRoleAssignment[] = [];
	let defaultUsersRole: number | undefined;
	let authProviders: AuthProvider[] = [];
	let authEnabled = false;

	switch (view) {
		case 'users':
			try {
				users = await UserService.listUsers({ fetch });
			} catch (err) {
				handleRouteError(err, `/users`, profile.current);
			}
			break;
		case 'groups':
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
				handleRouteError(err, '/admin/identity-access?view=groups', profile.current);
			}
			break;
		case 'roles':
			try {
				defaultUsersRole = await AdminService.getDefaultUsersRoleSettings({ fetch });
			} catch (err) {
				handleRouteError(err, `/admin/identity-access?view=roles`, profile.current);
			}
			break;
		case 'auth-providers':
			try {
				const version = await UserService.getVersion({ fetch });
				authEnabled = Boolean(version.authEnabled);
				if (authEnabled) {
					authProviders = await AdminService.listAuthProviders({ fetch });
				}
			} catch (err) {
				handleRouteError(err, '/admin/identity-access', profile.current);
			}
			break;
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
