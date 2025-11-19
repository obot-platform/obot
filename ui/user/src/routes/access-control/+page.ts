import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import type { AccessControlRule } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let accessControlRules: AccessControlRule[] = [];
	let workspaceId;

	try {
		workspaceId = await ChatService.fetchWorkspaceIDForProfile(profile.current?.id, { fetch });
		accessControlRules = await ChatService.listWorkspaceAccessControlRules(workspaceId, { fetch });
	} catch (err) {
		handleRouteError(err, '/access-control', profile.current);
	}

	return {
		accessControlRules,
		workspaceId
	};
};
