import { handleRouteError } from '$lib/errors';
import { AdminService, type SkillAccessPolicy } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	let skillAccessPolicy: SkillAccessPolicy | undefined;
	try {
		skillAccessPolicy = await AdminService.getSkillAccessPolicy(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/skill-access-policies/${id}`, profile);
	}

	return {
		skillAccessPolicy
	};
};
