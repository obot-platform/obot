import { ChatService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const project = await ChatService.getProject(params.project, { fetch });
	const tools = await ChatService.listTools(project.assistantID, project.id, { fetch });
	return {
		project,
		tools: tools.items
	};
};
