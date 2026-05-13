import { handleRouteError, HttpError } from '$lib/errors';
import { NanobotService } from '$lib/services';
import type { Skill } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent, params }) => {
	const { profile } = await parent();
	const { id } = params;
	let skill: Skill | undefined = undefined;
	let showLicenseError = false;

	try {
		skill = await NanobotService.getSkill(id, { fetch });
	} catch (err) {
		if (err instanceof HttpError && err.statusCode === 402) {
			skill = undefined;
			showLicenseError = true;
		} else {
			handleRouteError(err, `/admin/skills/${id}`, profile);
		}
	}

	return {
		skill,
		showLicenseError
	};
};
