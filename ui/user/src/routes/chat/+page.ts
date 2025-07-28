import { handleRouteError } from '$lib/errors';
import { ChatService, EditorService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let lastProjectId;
	try {
		const projects = (await ChatService.listProjects({ fetch })).items.sort(
			(a, b) => new Date(b.created).getTime() - new Date(a.created).getTime()
		);

		if (projects.length !== 0) {
			lastProjectId = projects[0].id;
		} else {
			// Create new project and redirect to it
			const newProject = await EditorService.createObot({ fetch });
			lastProjectId = newProject.id;
		}
	} catch (err) {
		// Handle redirecting to login, showing unauthorized error, etc.
		handleRouteError(err, '/chat', profile.current);
		return;
	}

	// Redirect to project
	throw redirect(302, `/o/${lastProjectId}`);
};
