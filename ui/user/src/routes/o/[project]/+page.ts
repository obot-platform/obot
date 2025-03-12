import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	try {
		const [project, toolReferences] = await Promise.all([
			ChatService.getProject(params.project, { fetch }),
			ChatService.listAllTools({ fetch })
		]);

		const [tools, assistant] = await Promise.all([
			ChatService.listTools(project.assistantID, project.id, { fetch }),
			ChatService.getAssistant(project.assistantID, { fetch })
		]);

		return { project, tools: tools.items, toolReferences: toolReferences.items, assistant };
	} catch (e) {
		handleRouteError(e, `/o/${params.project}`, profile.current);
	}
};
