import { handleRouteError } from '$lib/errors';
import { AdminService, NanobotService } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let agents: ProjectV2Agent[] = [];
	let users: OrgUser[] = [];
	try {
		[agents, users] = await Promise.all([
			NanobotService.listAllNanobotAgents({ fetch }),
			AdminService.listUsers({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, `/user-impersonation`, profile.current);
	}

	return {
		agents,
		users
	};
};
