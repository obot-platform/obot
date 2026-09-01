import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let modelAccessPolicy;
	try {
		modelAccessPolicy = await AdminService.getModelAccessPolicy(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/model-access-policies/${id}`, profile.current);
	}

	return {
		modelAccessPolicy
	};
};
