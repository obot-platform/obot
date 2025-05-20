import { handleRouteError } from '$lib/errors';
import {
	ChatService,
	type Project,
	type ProjectMCP,
	type ProjectShare,
	type ToolReference
} from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let share: ProjectShare | null = null;
	let project: Project | null = null;
	let mcps: ProjectMCP[] = [];
	let toolReferences = { items: [] as ToolReference[] };

	try {
		share = await ChatService.getProjectShareByPublicID(params.id, { fetch });
		toolReferences = await ChatService.listAllTools({ fetch });

		if (share?.projectID && share.editor) {
			// Get the project data
			project = await ChatService.getProject(share.projectID, { fetch });

			// Get tool references and MCPs in parallel
			if (project && project.assistantID) {
				mcps = await ChatService.listProjectMCPs(project.assistantID, project.id, { fetch });
			}
		}
	} catch (e) {
		handleRouteError(e, `/s/${params.id}`, profile.current);
	}

	return {
		id: params.id,
		featured: share?.featured ?? false,
		isOwner: share?.editor ?? false,
		projectID: share?.projectID,
		project,
		mcps,
		toolReferences: toolReferences.items
	};
};
