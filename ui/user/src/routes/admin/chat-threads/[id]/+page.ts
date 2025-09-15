import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let thread;
	try {
		thread = await AdminService.getThread(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/chat-threads/${id}`, profile.current);
	}

	return {
		thread
	};
};
