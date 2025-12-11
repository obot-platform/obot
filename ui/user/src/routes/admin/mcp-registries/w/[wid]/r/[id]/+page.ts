import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id, wid } = params;
	let accessControlRule;
	try {
		accessControlRule = await ChatService.getWorkspaceAccessControlRule(wid, id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/mcp-registries/w/${wid}/r/${id}`, profile.current);
	}

	return {
		accessControlRule,
		workspaceId: wid
	};
};
