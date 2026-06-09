import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry } from '../../../../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const catalogEntryId = params.id;
	const mcpServerId = params.ms_id;
	const { profile } = await parent();
	const prefix = profile.hasAdminAccess?.() ? '/admin' : '';
	const wid = url.searchParams.get('wid');

	let catalogEntry;
	try {
		catalogEntry = await getMCPCatalogEntry(catalogEntryId, wid, profile, fetch);
	} catch (err) {
		handleRouteError(
			err,
			`${prefix}/mcp-catalog/c/${catalogEntryId}/instance/${mcpServerId}`,
			profile
		);
	}

	return {
		catalogEntry,
		mcpServerId
	};
};
