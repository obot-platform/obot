import { handleRouteError } from '$lib/errors';
import { ChatService, type ProjectShare } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let share: ProjectShare | null = null;
	try {
		share = await ChatService.getProjectShareByPublicID(params.id, { fetch });
	} catch (e) {
		handleRouteError(e, `/s/${params.id}`, profile.current);
	}

	return {
		id: params.id,
		featured: share?.featured ?? false,
		isOwner: share?.editor ?? false,
		projectID: share?.projectID
	};
};
