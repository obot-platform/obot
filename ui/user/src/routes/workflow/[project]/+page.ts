import { handleRouteError } from '$lib/errors';
import { ChatService, type Project, type ProjectMCP } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let project: Project | undefined = undefined;
	let mcps: ProjectMCP[] = [];
	try {
		project = await ChatService.getProject(params.project, { fetch });
		mcps = await ChatService.listProjectMCPs(project.assistantID, project.id, { fetch });
	} catch (e) {
		handleRouteError(e, `/workflow/${params.project}`, profile.current);
	}

	return {
		project,
		mcps
	};
};
