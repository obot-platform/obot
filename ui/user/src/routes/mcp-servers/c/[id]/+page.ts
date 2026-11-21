import { handleRouteError } from '$lib/errors';
import { AdminService, type ToolListRequest } from '$lib/services';
import { getMCPCatalogEntry } from '../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { id } = params;

	const { profile } = await parent();
	let catalogEntry;
	try {
		catalogEntry = await getMCPCatalogEntry(id, url, profile, fetch);
	} catch (err) {
		handleRouteError(err, `/mcp-servers/c/${id}`, profile);
	}

	// The obot CLI sends the user here to confirm listing the entry's tools for their agent.
	let toolListRequest: ToolListRequest | undefined;
	const toolListRequestID = url.searchParams.get('toolListRequest');
	if (toolListRequestID) {
		toolListRequest = await AdminService.getToolListRequest(toolListRequestID, {
			fetch
		}).catch(() => undefined);
	}

	return {
		catalogEntry,
		toolListRequestID,
		toolListRequest
	};
};
