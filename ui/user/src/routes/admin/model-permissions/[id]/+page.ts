import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let modelPermissionRule;
	try {
		modelPermissionRule = await AdminService.getModelPermissionRule(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/model-permissions/${id}`, profile.current);
	}

	return {
		modelPermissionRule
	};
};
