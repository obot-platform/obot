import { DEFAULT_SYSTEM_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { MCPFilter, SystemMCPServerCatalogEntry } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let filters: MCPFilter[] = [];
	let systemCatalogEntries: SystemMCPServerCatalogEntry[] = [];

	try {
		filters = await AdminService.listMCPFilters({ fetch });
		systemCatalogEntries = await AdminService.listSystemMCPCatalogEntries(
			DEFAULT_SYSTEM_MCP_CATALOG_ID,
			{ fetch }
		);

		return { filters, systemCatalogEntries };
	} catch (err) {
		handleRouteError(err, '/admin/filters', profile.current);
	}
};
