import { ChatService, NanobotService } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: LayoutLoad = async ({ fetch, params }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
	}

	let agent: ProjectV2Agent;
	const agents = await NanobotService.listProjectV2Agents(params.id, { fetch });
	if (agents.length === 0) {
		agent = await NanobotService.createProjectV2Agent(
			params.id,
			{ displayName: 'New Agent' },
			{ fetch }
		);
	} else {
		agent = agents[0];
	}

	return {
		projectId: params.id,
		agent
	};
};
