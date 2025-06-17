import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { MCPCatalog } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let mcpCatalogs: MCPCatalog[] = [];

	try {
		mcpCatalogs = await AdminService.listMCPCatalogs({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/v2/mcp-catalogs', profile.current);
	}

	return {
		mcpCatalogs
	};
};
