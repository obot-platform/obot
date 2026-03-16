import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { SkillRepository } from '$lib/services/admin/types';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let skillRepositories: SkillRepository[] = [];
	let skills: Skill[] = [];

	try {
		skillRepositories = await AdminService.listSkillRepositories({ fetch });
		skills = await AdminService.listAllSkills({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/skills', profile);
	}

	return {
		skillRepositories,
		skills
	};
};
