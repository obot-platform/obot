import { handleRouteError } from '$lib/errors';
import { UserService, NanobotService, type OrgUser } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, version } = await parent();

	if (version?.agentsEnabled === false) {
		throw error(403, 'Obot Agent features are disabled.');
	}

	let agents: ProjectV2Agent[] = [];
	let users: OrgUser[] = [];
	try {
		[agents, users] = await Promise.all([
			NanobotService.listAllNanobotAgents({ fetch }),
			UserService.listUsers({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, `/agents`, profile);
	}

	return {
		agents,
		users
	};
};
