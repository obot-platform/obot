import { handleRouteError } from '$lib/errors';
import { getMCPCatalogServer } from '../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();
	let mcpServer;
	try {
		mcpServer = await getMCPCatalogServer(id, url, profile, fetch);
	} catch (err) {
		handleRouteError(err, `/mcp-servers/s/${id}/details`, profile);
	}

	return {
		mcpServer,
		id
	};
};
