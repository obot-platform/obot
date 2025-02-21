import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const agentID = params.agent;
	const projects = await ChatService.listProjects({ fetch });
	let project = projects.items.find((p) => p.assistantID === agentID || p.id === agentID);
	if (!project) {
		const agent = await ChatService.getAssistant(agentID, { fetch });
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
