import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry } from '../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { id } = params;
	const wid = url.searchParams.get('wid');

	const { profile } = await parent();
	let catalogEntry;
	try {
		catalogEntry = await getMCPCatalogEntry(id, wid, profile, fetch);
	} catch (err) {
		handleRouteError(err, `/mcp-servers/c/${id}`, profile);
	}

	return {
		catalogEntry
	};
};
