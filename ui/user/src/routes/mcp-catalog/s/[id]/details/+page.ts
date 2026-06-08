import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();
	const prefix = profile.hasAdminAccess?.() ? '/admin' : '';

	let mcpServer;
	try {
		mcpServer = await UserService.getMcpCatalogServer(id, { fetch });
	} catch (err) {
		handleRouteError(err, `${prefix}/mcp-catalog/s/${id}/details`, profile);
	}

	return {
		mcpServer,
		id
	};
};
