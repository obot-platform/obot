import { handleRouteError, HttpError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { GitCredential, SkillAccessPolicy, SkillRepository } from '$lib/services/admin/types';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

const views = new Set(['skills', 'sources', 'access-policies']);

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	const requestedView = url.searchParams.get('view');
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const view =
		requestedView && views.has(requestedView) && (hasAdminAccess || requestedView === 'skills')
			? requestedView
			: 'skills';

	let skillRepositories: SkillRepository[] = [];
	let skills: Skill[] = [];
	let gitCredentials: GitCredential[] = [];
	let skillAccessPolicies: SkillAccessPolicy[] = [];
	let showLicenseError = false;

	if (view === 'skills') {
		if (hasAdminAccess) {
			try {
				[skillRepositories, gitCredentials] = await Promise.all([
					AdminService.listSkillRepositories({ fetch, dontLogErrors: true }),
					AdminService.listGitCredentials({ fetch, dontLogErrors: true }).catch(() => [])
				]);
			} catch (err) {
				handleRouteError(err, '/skills', profile);
			}
		}

		try {
			skills = hasAdminAccess
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
	} else if (hasAdminAccess) {
		switch (view) {
			case 'sources':
				try {
					[skillRepositories, gitCredentials] = await Promise.all([
						AdminService.listSkillRepositories({ fetch, dontLogErrors: true }),
						AdminService.listGitCredentials({ fetch, dontLogErrors: true }).catch(() => [])
					]);
				} catch (err) {
					handleRouteError(err, '/skills', profile);
				}
				break;
			case 'access-policies':
				try {
					skillAccessPolicies = await AdminService.listSkillAccessPolicies({ fetch });
				} catch (err) {
					handleRouteError(err, '/skills', profile);
				}
				break;
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
