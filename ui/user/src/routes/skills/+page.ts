import { handleRouteError, HttpError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { GitCredential, SkillAccessPolicy, SkillRepository } from '$lib/services/admin/types';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	let skillRepositories: SkillRepository[] = [];
	let skills: Skill[] = [];
	let gitCredentials: GitCredential[] = [];
	let skillAccessPolicies: SkillAccessPolicy[] = [];
	let showLicenseError = false;

	const view = url.searchParams.get('view');
	const isSkillsView = view !== 'sources' && view !== 'access-policy' && view !== 'git-credentials';

	if (profile.hasAdminAccess?.()) {
		try {
			[skillRepositories, gitCredentials] = await Promise.all([
				AdminService.listSkillRepositories({ fetch, dontLogErrors: true }),
				AdminService.listGitCredentials({ fetch, dontLogErrors: true }).catch(() => [])
			]);
		} catch (err) {
			handleRouteError(err, '/skills', profile);
		}
	}

	if (isSkillsView) {
		try {
			skills = profile.hasAdminAccess?.()
				? await AdminService.listAllSkills({ fetch, dontLogErrors: true })
				: await UserService.listSkills({ fetch, dontLogErrors: true });
		} catch (err) {
			if (err instanceof HttpError && err.statusCode === 402) {
				skills = [];
				showLicenseError = true;
			} else {
				handleRouteError(err, '/skills', profile);
			}
		}
	} else if (profile.hasAdminAccess?.()) {
		try {
			skillAccessPolicies = await AdminService.listSkillAccessPolicies({ fetch });
		} catch (err) {
			handleRouteError(err, '/skills', profile);
		}
	}

	return {
		skillRepositories,
		gitCredentials,
		skills,
		showLicenseError,
		skillAccessPolicies
	};
};
