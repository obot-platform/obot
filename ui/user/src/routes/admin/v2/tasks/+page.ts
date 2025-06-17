import { handleRouteError } from '$lib/errors';
import { AdminService, type Project, type Task } from '$lib/services';
import type { OrgUser, ProjectThread } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let tasks: Task[] = [];
	let threads: ProjectThread[] = [];
	let projects: Project[] = [];
	let users: OrgUser[] = [];
	try {
		tasks = await AdminService.listTasks({ fetch });
		threads = await AdminService.listThreads({ fetch });
		users = await AdminService.listUsers({ fetch });
		projects = await AdminService.listProjects({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/v2/tasks', profile.current);
	}

	return {
		tasks,
		threads,
		users,
		projects
	};
};
