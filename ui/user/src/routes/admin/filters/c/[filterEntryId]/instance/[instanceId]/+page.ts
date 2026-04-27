import { DEFAULT_SYSTEM_MCP_CATALOG_ID } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	const { filterEntryId, instanceId } = params;

	try {
		const entry = await AdminService.getSystemMCPCatalogEntry(
			DEFAULT_SYSTEM_MCP_CATALOG_ID,
			filterEntryId,
			{
				fetch
			}
		);
		const filter = await AdminService.getMCPFilter(instanceId, {
			fetch
		});
		return { entry, filter };
	} catch (err) {
		handleRouteError(err, `/admin/filters/c/${filterEntryId}/instance/${instanceId}`, profile);
	}
};
