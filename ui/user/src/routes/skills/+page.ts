import { handleRouteError } from '$lib/errors';
import { NanobotService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let skills;

	try {
		skills = await NanobotService.listSkills({ fetch });
	} catch (err) {
		handleRouteError(err, `/skills`, profile);
	}

	return {
		skills
	};
};
