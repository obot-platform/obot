import { handleRouteError } from '$lib/errors';
import { AdminService, type Project } from '$lib/services';
import type { OrgUser, ProjectThread } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let projects: Project[] = [];
	let threads: ProjectThread[] = [];
	let users: OrgUser[] = [];
	try {
		projects = await AdminService.listProjects({ fetch });
		threads = await AdminService.listThreads({ fetch });
		users = await AdminService.listUsers({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/threads', profile.current);
	}

	return {
		projects,
		threads,
		users
	};
};
