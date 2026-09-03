import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry } from '../../../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	const { profile } = await parent();

	let catalogEntry;
	try {
		catalogEntry = await getMCPCatalogEntry(catalogEntryId, url, profile, fetch);
	} catch (err) {
		handleRouteError(
			err,
			`/mcp-servers/c/${catalogEntryId}/instance/${mcpServerId}/details`,
			profile
		);
	}

	return {
		catalogEntry,
		mcpServerId
	};
};
