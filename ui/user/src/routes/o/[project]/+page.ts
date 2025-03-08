import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { handleRouteError } from '$lib/errors';

export const load: PageLoad = async ({ params, fetch }) => {
	try {
		const project = await ChatService.getProject(params.project, { fetch });
		const tools = await ChatService.listTools(project.assistantID, project.id, { fetch });
		return {
			project,
			tools: tools.items
		};
	} catch (e) {
		handleRouteError(e, `/o/${params.project}`, profile.current);
	}
};
