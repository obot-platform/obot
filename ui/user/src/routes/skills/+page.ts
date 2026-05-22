import { handleRouteError } from '$lib/errors';
import { NanobotService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let skills;

	if (profile?.hasAdminAccess?.()) {
		throw redirect(302, '/admin/skills');
	}

	try {
		skills = await NanobotService.listSkills({ fetch });
	} catch (err) {
		handleRouteError(err, `/skills`, profile);
	}

	return {
		skills
	};
};
