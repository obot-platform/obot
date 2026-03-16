import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { SkillAccessPolicy } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let skillAccessPolicies: SkillAccessPolicy[] = [];

	try {
		skillAccessPolicies = await AdminService.listSkillAccessPolicies({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/skill-access-policies', profile);
	}

	return {
		skillAccessPolicies
	};
};
