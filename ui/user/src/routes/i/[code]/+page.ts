import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

// Disable server-side rendering for this route
export const ssr = false;

export const load = (async ({ params, fetch }) => {
	try {
		const invitation = await ChatService.getProjectInvitation(params.code, { fetch });
		return { invitation };
	} catch (e) {
		handleRouteError(e, `/i/${params.code}`, profile.current);
	}
}) satisfies PageLoad;
