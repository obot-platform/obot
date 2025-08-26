import { handleRouteError } from '$lib/errors';
import { AdminService, type Project, type Task, type Thread } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const { id } = params;

	let taskRun: Thread | null = null;
	let task: Task | null = null;
	let project: Project | null = null;
	try {
		taskRun = await AdminService.getThread(id, { fetch });
		if (taskRun?.taskID) {
			task = await AdminService.getTask(taskRun.taskID, { fetch });
		}
		if (task?.projectID) {
			project = await AdminService.getProject(task.projectID, { fetch });
		}
	} catch (err) {
		handleRouteError(err, `/tasks/${id}`, profile.current);
	}

	return {
		taskRun,
		task,
		project
	};
};
