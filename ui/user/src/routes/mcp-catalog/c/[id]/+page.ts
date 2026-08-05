import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { getMCPCatalogEntry } from '../../utils';
import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';

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

	let catalogVersion;
	const versionParam = url.searchParams.get('version');
	if (versionParam !== null) {
		const version = Number(versionParam);
		if (!Number.isInteger(version) || version < 0) {
			throw error(400, 'Catalog version must be a non-negative integer');
		}
		if (!profile.hasAdminAccess?.() || wid || !catalogEntry) {
			throw error(404, 'Catalog version not found');
		}

		try {
			catalogVersion = await AdminService.getMCPCatalogEntryVersion(
				DEFAULT_MCP_CATALOG_ID,
				id,
				version,
				{ fetch }
			);
		} catch (err) {
			handleRouteError(err, `${prefix}/mcp-catalog/c/${id}?version=${version}`, profile);
		}
	}

	return {
		catalogEntry,
		catalogVersion
	};
};
