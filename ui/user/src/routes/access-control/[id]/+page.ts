import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let accessControlRule;
	let workspaceId;
	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		accessControlRule = await ChatService.getWorkspaceAccessControlRule(workspaceId, id, { fetch });
	} catch (err) {
		handleRouteError(err, `/access-control/${id}`, profile.current);
	}

	return {
		accessControlRule,
		workspaceId
	};
};
