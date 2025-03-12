import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	try {
		const project = await ChatService.getProject(params.project, { fetch });
		const tools = await ChatService.listTools(project.assistantID, project.id, { fetch });
		const toolReferences = await ChatService.listAllTools({ fetch });
		return {
			project,
			tools: tools.items,
			toolReferences: toolReferences.items
		};
	} catch (e) {
		handleRouteError(e, `/o/${params.project}`, profile.current);
	}
};
