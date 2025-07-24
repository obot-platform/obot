import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let filter;
	try {
		filter = await AdminService.getMCPFilter(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/v2/admin/filters/${id}`, profile.current);
	}

	return {
		filter
	};
};
