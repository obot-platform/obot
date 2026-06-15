import { handleRouteError, HttpError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { SkillRepository } from '$lib/services/admin/types';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	let skillRepositories: SkillRepository[] = [];
	let skills: Skill[] = [];
	let showLicenseError = false;

	const viewParam = url.searchParams.get('view');
	const isSkillsView = (viewParam ?? 'urls') !== 'urls';

	try {
		skillRepositories = await AdminService.listSkillRepositories({ fetch, dontLogErrors: true });
	} catch (err) {
		handleRouteError(err, '/admin/skills', profile);
	}

	if (isSkillsView) {
		try {
			skills = await AdminService.listAllSkills({ fetch, dontLogErrors: true });
		} catch (err) {
			if (err instanceof HttpError && err.statusCode === 402) {
				skills = [];
				showLicenseError = true;
			} else {
				handleRouteError(err, '/admin/skills', profile);
			}
		}
	}

	return {
		skillRepositories,
		skills,
		showLicenseError
	};
};
