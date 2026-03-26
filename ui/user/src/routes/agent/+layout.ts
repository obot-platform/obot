import { ChatService, NanobotService } from '$lib/services';
import type { ProjectV2Agent } from '$lib/services/nanobot/types';
import type { LayoutLoad } from './$types';
import { error, redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: LayoutLoad = async ({ fetch, url, parent }) => {
	const { profile } = await parent();
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
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
		const project = await NanobotService.getProjectV2(targetProjectId, { fetch });
		const agents = await NanobotService.listProjectV2Agents(targetProjectId, { fetch });
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
