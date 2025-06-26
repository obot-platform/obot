import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AccessControlRule } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let accessControlRules: AccessControlRule[] = [];

	try {
		accessControlRules = await AdminService.listAccessControlRules({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/access-control', profile.current);
	}

	return {
		accessControlRules
	};
};
