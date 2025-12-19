import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let modelAccessPolicies: ModelAccessPolicy[] = [];

	try {
		modelAccessPolicies = await AdminService.listModelAccessPolicies({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/model-access-policies', profile.current);
	}

	return {
		modelAccessPolicies
	};
};
