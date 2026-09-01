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
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent, depends }) => {
	depends('mcp-access-policies:data');

	const { profile } = await parent();

	const isPowerUserOrAdmin = profile.groups.includes(Group.POWERUSER) || profile.hasAdminAccess?.();

	if (!isPowerUserOrAdmin) {
		throw redirect(302, '/mcp-servers');
	}

	let gitCredentials: GitCredential[] = [];
	let filters: MCPFilter[] = [];
	let systemCatalogEntries: SystemMCPServerCatalogEntry[] = [];
	let mcpTunnels: MCPTunnel[] = [];
	let tunnelConnections: TunnelConnection[] | undefined;
	let accessControlRules: AccessControlRule[] = [];

	if (profile.hasAdminAccess?.()) {
		gitCredentials = await AdminService.listGitCredentials({ fetch, dontLogErrors: true }).catch(
			() => []
		);
		filters = await AdminService.listMCPFilters({ fetch }).catch(() => []);
		systemCatalogEntries = await AdminService.listSystemMCPCatalogEntries(
			DEFAULT_SYSTEM_MCP_CATALOG_ID,
			{ fetch }
		).catch(() => []);
		mcpTunnels = await AdminService.listMCPTunnels({ fetch }).catch(() => []);
		tunnelConnections = await AdminService.listTunnelConnections({
			fetch,
			dontLogErrors: true
		}).catch(() => undefined);

		try {
			const adminAccessControlRules = await AdminService.listAccessControlRules({ fetch });
			const userWorkspacesAccessControlRules =
				await AdminService.listAllUserWorkspaceAccessControlRules({ fetch });
			accessControlRules = [...adminAccessControlRules, ...userWorkspacesAccessControlRules];
		} catch (err) {
			handleRouteError(err, '/mcp-servers', profile);
		}
	}

	try {
		const workspaceId = await UserService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		if (!profile.hasAdminAccess?.()) {
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
