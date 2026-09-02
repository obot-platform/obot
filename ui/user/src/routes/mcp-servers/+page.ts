import { DEFAULT_SYSTEM_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	Group,
	UserService,
	type AccessControlRule,
	type GitCredential,
	type MCPFilter,
	type MCPTunnel,
	type SystemMCPServerCatalogEntry,
	type TunnelConnection
} from '$lib/services';
import type { PageLoad } from './$types';

const views = new Set([
	'servers',
	'sources',
	'git-credentials',
	'deployments',
	'filters',
	'tunnels',
	'access-policy'
]);

export const load: PageLoad = async ({ fetch, parent, depends, url }) => {
	const { profile } = await parent();
	const requestedView = url.searchParams.get('view');
	const view = requestedView && views.has(requestedView) ? requestedView : 'servers';

	const isPowerUserOrAdmin = profile.groups.includes(Group.POWERUSER) || profile.hasAdminAccess?.();

	let gitCredentials: GitCredential[] = [];
	let filters: MCPFilter[] = [];
	let systemCatalogEntries: SystemMCPServerCatalogEntry[] = [];
	let mcpTunnels: MCPTunnel[] = [];
	let tunnelConnections: TunnelConnection[] | undefined;
	let accessControlRules: AccessControlRule[] = [];

	if (profile.hasAdminAccess?.()) {
		switch (view) {
			case 'sources':
			case 'git-credentials':
				gitCredentials = await AdminService.listGitCredentials({
					fetch,
					dontLogErrors: true
				}).catch(() => []);
				break;
			case 'filters':
				[filters, systemCatalogEntries] = await Promise.all([
					AdminService.listMCPFilters({ fetch }).catch(() => []),
					AdminService.listSystemMCPCatalogEntries(DEFAULT_SYSTEM_MCP_CATALOG_ID, {
						fetch
					}).catch(() => [])
				]);
				break;
			case 'tunnels':
				[mcpTunnels, tunnelConnections] = await Promise.all([
					AdminService.listMCPTunnels({ fetch }).catch(() => []),
					AdminService.listTunnelConnections({
						fetch,
						dontLogErrors: true
					}).catch(() => undefined)
				]);
				break;
			case 'access-policy':
				depends('mcp-access-policies:data');
				try {
					const [adminAccessControlRules, userWorkspacesAccessControlRules] = await Promise.all([
						AdminService.listAccessControlRules({ fetch }),
						AdminService.listAllUserWorkspaceAccessControlRules({ fetch })
					]);
					accessControlRules = [...adminAccessControlRules, ...userWorkspacesAccessControlRules];
				} catch (err) {
					handleRouteError(err, '/mcp-servers', profile);
				}
				break;
		}
	}

	const needsWorkspace = !profile.hasAdminAccess?.() && ['servers', 'access-policy'].includes(view);
	if (!needsWorkspace) {
		return {
			workspaceId: undefined,
			gitCredentials,
			filters,
			systemCatalogEntries,
			mcpTunnels,
			tunnelConnections,
			accessControlRules
		};
	}

	try {
		const workspaceId = await UserService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		if (view === 'access-policy' && isPowerUserOrAdmin) {
			depends('mcp-access-policies:data');
			try {
				accessControlRules = await UserService.listWorkspaceAccessControlRules(workspaceId, {
					fetch
				});
			} catch (err) {
				handleRouteError(err, '/mcp-servers', profile);
			}
		}
		return {
			workspaceId,
			gitCredentials,
			filters,
			systemCatalogEntries,
			mcpTunnels,
			tunnelConnections,
			accessControlRules
		};
	} catch (_err) {
		return {
			workspaceId: undefined,
			gitCredentials,
			filters,
			systemCatalogEntries,
			mcpTunnels,
			tunnelConnections,
			accessControlRules
		};
	}
};
