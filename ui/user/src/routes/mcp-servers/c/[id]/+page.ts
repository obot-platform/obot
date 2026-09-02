import { handleRouteError } from '$lib/errors';
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

	return {
		catalogEntry
	};
};
