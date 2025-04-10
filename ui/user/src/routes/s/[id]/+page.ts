import { handleRouteError } from '$lib/errors';
import { ChatService, type Project, type ProjectShare } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let share: ProjectShare | null = null;
	try {
		share = await ChatService.getProjectShareByPublicID(params.id, { fetch });
	} catch (e) {
		handleRouteError(e, `/s/${params.id}`, profile.current);
	}
	let project: Project | null = null;
	if (share?.projectID) {
		try {
			project = await ChatService.getProject(share.projectID, { fetch });
		} catch (_error) {
			// do nothing
		}
	}

	return {
		id: params.id,
		featured: share?.featured ?? false,
		isOwner: project?.editor ?? false
	};
};
