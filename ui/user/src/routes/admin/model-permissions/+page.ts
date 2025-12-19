import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { ModelPermissionRule } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let modelPermissionRules: ModelPermissionRule[] = [];

	try {
		modelPermissionRules = await AdminService.listModelPermissionRules({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/model-permissions', profile.current);
	}

	return {
		modelPermissionRules
	};
};
