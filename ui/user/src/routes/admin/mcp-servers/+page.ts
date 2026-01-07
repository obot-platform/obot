import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	try {
		const workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.id, { fetch });
		return {
			workspaceId
		};
	} catch (_err) {
		// ex. may not have a workspaceId if basic user with auditor access
		return {
			workspaceId: undefined
		};
	}
};
