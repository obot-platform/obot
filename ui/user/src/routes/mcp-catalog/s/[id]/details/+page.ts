import { handleRouteError } from '$lib/errors';
import { getMCPCatalogServer } from '../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { id } = params;
	const wid = url.searchParams.get('wid');
	const { profile } = await parent();
	const prefix = profile.hasAdminAccess?.() ? '/admin' : '';

	let mcpServer;
	try {
		mcpServer = await getMCPCatalogServer(id, wid, profile, fetch);
	} catch (err) {
		handleRouteError(err, `${prefix}/mcp-catalog/s/${id}/details`, profile);
	}

	return {
		mcpServer,
		id
	};
};
