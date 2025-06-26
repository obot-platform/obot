import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let accessControlRule;
	try {
		accessControlRule = await AdminService.getAccessControlRule(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/v2/admin/access-control/${id}`, profile.current);
	}

	return {
		accessControlRule
	};
};
