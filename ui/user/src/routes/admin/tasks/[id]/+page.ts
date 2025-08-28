import { handleRouteError } from '$lib/errors';
import { AdminService, type Project, type Task } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const { id } = params;

	let task: Task | null = null;
	let project: Project | null = null;
	try {
		task = await AdminService.getTask(id, { fetch });
		if (task?.projectID) {
			project = await AdminService.getProject(task.projectID, { fetch });
		}
	} catch (err) {
		handleRouteError(err, `/tasks/${id}`, profile.current);
	}

	return {
		task,
		project
	};
};
