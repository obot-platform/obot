import { ChatService, NanobotService } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
	}

	let projects = await NanobotService.listProjectsV2({ fetch });
	if (projects.length === 0) {
		const project = await NanobotService.createProjectV2({ displayName: 'New Project' }, { fetch });
		projects = [project];
	}

	let agent: ProjectV2Agent;
	let isNewAgent = false;
	const agents = await NanobotService.listProjectV2Agents(projects[0].id, { fetch });
	if (agents.length === 0) {
		agent = await NanobotService.createProjectV2Agent(
			projects[0].id,
			{ displayName: 'New Agent' },
			{ fetch }
		);
		isNewAgent = true;
	} else {
		agent = agents[0];
	}

	return { projects, agent, isNewAgent };
};
