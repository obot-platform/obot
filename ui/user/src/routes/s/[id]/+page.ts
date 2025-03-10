import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const id = params.id;
	try {
		const project = await ChatService.createProjectFromShare(id, { fetch });
		return {
			project
		};
	} catch (e) {
		handleRouteError(e, `/s/${id}`, profile.current);
	}
};
