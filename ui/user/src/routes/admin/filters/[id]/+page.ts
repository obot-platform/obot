import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	try {
		const filter = await AdminService.getMCPFilter(id, { fetch });
		return { filter };
	} catch (err) {
		handleRouteError(err, `/admin/filters/${id}`, profile);
	}
};
