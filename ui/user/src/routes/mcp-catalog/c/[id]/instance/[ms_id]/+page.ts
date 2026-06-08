import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	const { profile } = await parent();
	const prefix = profile.hasAdminAccess?.() ? '/admin' : '';

	let catalogEntry;
	let mcpServer;
	try {
		catalogEntry = await UserService.getMCP(catalogEntryId, {
			fetch
		});
		mcpServer = await UserService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
	} catch (err) {
		handleRouteError(
			err,
			`${prefix}/mcp-catalog/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile
		);
	}

	return {
		catalogEntry,
		mcpServerId,
		mcpServer
	};
};
