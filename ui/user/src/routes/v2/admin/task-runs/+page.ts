import { handleRouteError } from '$lib/errors';
import { AdminService, type Task } from '$lib/services';
import type { ProjectThread } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let tasks: Task[] = [];
	let threads: ProjectThread[] = [];
	try {
		tasks = await AdminService.listTasks({ fetch });
		threads = await AdminService.listThreads({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/task-runs', profile.current);
	}

	return {
		tasks,
		threads
	};
};
