import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const agentID = params.agent;
	const agent = await ChatService.getAssistant(agentID, { fetch });
	const projects = await ChatService.listProjects({ fetch });
	let project = projects.items.find((p) => p.assistantID === agent.id);
	if (!project) {
		project = await ChatService.createProject(agent.id, {
			name: agent.name,
			description: agent.description,
			fetch
		});
	}
	return {
		project
	};
};
