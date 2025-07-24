import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { MCPFilter } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let filters: MCPFilter[] = [];

	try {
		filters = await AdminService.listMCPFilters({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/filters', profile.current);
	}

	return {
		filters
	};
};
