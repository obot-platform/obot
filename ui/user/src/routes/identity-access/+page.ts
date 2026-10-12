import { AUTH_PROVIDERS_VIEW_PATH, isSCIMView, SCIM_VIEW_PATH } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService, ApiKeysService, UserService } from '$lib/services';
import type { AuthProvider, GroupRoleAssignment, OrgGroup, OrgUser } from '$lib/services';
import type { SCIMConnectionReview, SCIMEnablePreview } from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import type { PageLoad } from './$types';

const views = new Set(['users', 'agents', 'groups', 'roles', 'auth-providers']);

// The page size of each list on the SCIM sub-tab.
const scimPageSize = 50;

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const requestedView = url.searchParams.get('view');
	const view =
		requestedView && views.has(requestedView) && (hasAdminAccess || requestedView === 'agents')
			? requestedView
			: hasAdminAccess
				? 'users'
				: 'agents';

	let users: OrgUser[] = [];
	let groups: OrgGroup[] = [];
	let groupRoleAssignments: GroupRoleAssignment[] = [];
	let defaultUsersRole: number | undefined;
	let authProviders: AuthProvider[] = [];
	let authEnabled = false;
	let apiKeys: APIKey[] = [];
	let scimReview: SCIMConnectionReview | undefined;
	let scimEnablePreview: SCIMEnablePreview | undefined;

	if (view === 'agents') {
		try {
			if (hasAdminAccess) {
				[apiKeys, users] = await Promise.all([
					ApiKeysService.listAllApiKeys({ fetch }),
					UserService.listUsers({ fetch })
				]);
			} else {
				apiKeys = await ApiKeysService.listApiKeys({ fetch });
			}
		} catch (err) {
			handleRouteError(err, '/identity-access?view=agents', profile);
		}
	} else if (hasAdminAccess) {
		switch (view) {
			case 'users':
				try {
					users = await UserService.listUsers({ fetch });
				} catch (err) {
					handleRouteError(err, `/identity-access?view=users`, profile);
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
					handleRouteError(err, '/identity-access?view=groups', profile);
				}
				break;
			case 'roles':
				try {
					defaultUsersRole = await AdminService.getDefaultUsersRoleSettings({ fetch });
				} catch (err) {
					handleRouteError(err, `/identity-access?view=roles`, profile);
				}
				break;
			case 'auth-providers':
				if (isSCIMView(url.searchParams)) {
					try {
						// There is at most one connection.
						const connections = await AdminService.listSCIMConnections({ fetch });
						if (connections.length > 0) {
							scimReview = await AdminService.getSCIMConnectionReview(connections[0].id, {
								fetch,
								limit: scimPageSize
							});
						} else {
							// Without a connection, the configured provider may synchronize its directory at
							// sign-in, and can then be moved to SCIM.
							scimEnablePreview = await AdminService.getSCIMEnablePreview({ fetch });
						}
					} catch (err) {
						handleRouteError(err, SCIM_VIEW_PATH, profile);
					}
				} else {
					try {
						const version = await UserService.getVersion({ fetch });
						authEnabled = Boolean(version.authEnabled);
						if (authEnabled) {
							authProviders = await AdminService.listAuthProviders({ fetch });
						}
					} catch (err) {
						handleRouteError(err, AUTH_PROVIDERS_VIEW_PATH, profile);
					}
				}
				break;
		}
	}

	return {
		users,
		groups,
		groupRoleAssignments,
		defaultUsersRole,
		authProviders,
		authEnabled,
		apiKeys,
		scimReview,
		scimEnablePreview,
		scimPageSize
	};
};
