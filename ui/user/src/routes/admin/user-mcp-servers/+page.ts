import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { WorkspaceCatalogEntry, WorkspaceCatalogServer } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let servers: WorkspaceCatalogServer[] = [];
	let entries: WorkspaceCatalogEntry[] = [];
	try {
		servers = await AdminService.listAllUserWorkspaceMCPServers({ fetch });
		entries = await AdminService.listAllUserWorkspaceCatalogEntries({ fetch });
	} catch (err) {
		handleRouteError(err, `/users`, profile.current);
	}

	return {
		servers,
		entries
	};
};
