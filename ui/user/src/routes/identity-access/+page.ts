import { AUTH_PROVIDERS_VIEW_PATH, isSCIMView, SCIM_VIEW_PATH } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService, AgentIdentitiesService, ApiKeysService, UserService } from '$lib/services';
import type { AuthProvider, GroupRoleAssignment, OrgGroup, OrgUser } from '$lib/services';
import type { SCIMConnectionReview, SCIMEnablePreview } from '$lib/services/admin/types';
import type { AgentIdentity } from '$lib/services/agent-identities/types';
import type { APIKey } from '$lib/services/api-keys/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// 'agents' was the tab of a user's API keys. It is 'api-keys' now, and 'agent-identities' is the
// admin tab of agent identities.
const views = new Set([
	'users',
	'api-keys',
	'agent-identities',
	'groups',
	'roles',
	'auth-providers'
]);

// The page size of each list on the SCIM sub-tab.
const scimPageSize = 50;

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	if (url.searchParams.get('view') === 'agents') {
		const redirected = new URL(url);
		redirected.searchParams.set('view', 'api-keys');
		throw redirect(301, `${redirected.pathname}${redirected.search}`);
	}
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const requestedView = url.searchParams.get('view');
	const view =
		requestedView && views.has(requestedView) && (hasAdminAccess || requestedView === 'api-keys')
			? requestedView
			: hasAdminAccess
				? 'users'
				: 'api-keys';

	let users: OrgUser[] = [];
	let groups: OrgGroup[] = [];
	let groupRoleAssignments: GroupRoleAssignment[] = [];
	let defaultUsersRole: number | undefined;
	let authProviders: AuthProvider[] = [];
	let authEnabled = false;
	let apiKeys: APIKey[] = [];
	let agents: AgentIdentity[] = [];
	let scimReview: SCIMConnectionReview | undefined;
	let scimEnablePreview: SCIMEnablePreview | undefined;

	if (view === 'api-keys') {
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
			handleRouteError(err, '/identity-access?view=api-keys', profile);
		}
	} else if (hasAdminAccess) {
		switch (view) {
			case 'agent-identities':
				try {
					[agents, users] = await Promise.all([
						AgentIdentitiesService.listAgents({ fetch }),
						UserService.listUsers({ fetch })
					]);
				} catch (err) {
					handleRouteError(err, '/identity-access?view=agent-identities', profile);
				}
				break;
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
		agents,
		scimReview,
		scimEnablePreview,
		scimPageSize
	};
};
