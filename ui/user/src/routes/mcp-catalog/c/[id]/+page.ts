import { handleRouteError } from '$lib/errors';
import { getMCPCatalogEntry } from '../../utils';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, url, fetch, parent }) => {
	const { id } = params;
	const wid = url.searchParams.get('wid');

	const { profile } = await parent();
	const prefix = profile.hasAdminAccess?.() ? '/admin' : '';

	let catalogEntry;
	try {
		catalogEntry = await getMCPCatalogEntry(id, wid, profile, fetch);
	} catch (err) {
		handleRouteError(err, `${prefix}/mcp-catalog/c/${id}`, profile);
	}

	return {
		catalogEntry
	};
};
