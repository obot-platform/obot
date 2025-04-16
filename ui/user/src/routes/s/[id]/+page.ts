import { handleRouteError } from '$lib/errors';
import { ChatService, type Project, type ProjectShare } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch }) => {
	let share: ProjectShare | null = null;
	try {
		share = await ChatService.getProjectShareByPublicID(params.id, { fetch });
	} catch (e) {
		handleRouteError(e, `/s/${params.id}`, profile.current);
	}

	// If the user received projectID containing the params.id / shareID,
	// they're receiving their obot instance project ID
	if (share?.projectID.split('-').includes(params.id)) {
		// redirect to their obot instance project
		throw redirect(303, `/o/${share?.projectID}`);
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
