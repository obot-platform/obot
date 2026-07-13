import { handleRouteError } from '$lib/errors';
import { NanobotService } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { LayoutLoad } from './$types';
import { error } from '@sveltejs/kit';

export const ssr = false;

export const load: LayoutLoad = async ({ fetch, url, parent }) => {
	const { profile, version } = await parent();

	if (version?.agentsEnabled === false) {
		throw error(403, 'Obot Agent features are disabled.');
	}

	// Check for an explicit project ID from query params or URL path.
	// This allows impersonation: navigating directly to another user's project.
	let targetProjectId = url.searchParams.get('projectId');
	const targetAgentId = url.searchParams.get('agentId');

	if (!targetProjectId) {
		const match = url.pathname.match(/^\/agent\/p\/([^/]+)/);
		if (match) targetProjectId = match[1];
	}

	if (targetProjectId) {
		let project;
		let agents;
		try {
			project = await NanobotService.getProject(targetProjectId, { fetch });
			agents = await NanobotService.listProjectAgents(targetProjectId, { fetch });
		} catch (err) {
			handleRouteError(err, url.pathname, profile);
		}

		let agent: ProjectV2Agent;
		if (targetAgentId) {
			agent = agents.find((a) => a.id === targetAgentId) || agents[0];
		} else {
			agent = agents[0];
		}

		if (agent.userID !== profile.id && !profile.canImpersonate?.()) {
			throw error(403, 'You do not have permission to view this agent.');
		}

		return { projects: [project], agent, isNewAgent: false };
	}

	// Default: load or create the current user's project and agent.
	let projects;
	let agent: ProjectV2Agent;
	let isNewAgent = false;
	try {
		projects = await NanobotService.listProjects({ fetch });
		if (projects.length === 0) {
			const project = await NanobotService.createProject({ displayName: 'New Project' }, { fetch });
			projects = [project];
		}

		const agents = await NanobotService.listProjectAgents(projects[0].id, { fetch });
		if (agents.length === 0) {
			agent = await NanobotService.createProjectAgent(
				projects[0].id,
				{ displayName: 'New Agent' },
				{ fetch }
			);
			isNewAgent = true;
		} else {
			agent = agents[0];
		}
	} catch (err) {
		handleRouteError(err, url.pathname, profile);
	}

	return { projects, agent, isNewAgent };
};
