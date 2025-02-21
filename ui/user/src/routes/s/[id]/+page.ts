import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const id = params.id;
	return {
		project: await ChatService.createProjectFromShare(id, { fetch })
	};
};
