import { handleRouteError } from '$lib/errors';
import { AdminService, NanobotService } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let agents: ProjectV2Agent[] = [];
	let users: OrgUser[] = [];
	try {
		[agents, users] = await Promise.all([
			NanobotService.listAllNanobotAgents({ fetch }),
			AdminService.listUsers({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, `/agents`, profile);
	}

	return {
		agents,
		users
	};
};
