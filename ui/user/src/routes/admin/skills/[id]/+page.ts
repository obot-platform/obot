import { handleRouteError } from '$lib/errors';
import { NanobotService } from '$lib/services';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent, params }) => {
	const { profile } = await parent();
	const { id } = params;
	let skill: Skill | undefined = undefined;

	try {
		skill = await NanobotService.getSkill(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/skills/${id}`, profile);
	}

	return {
		skill
	};
};
