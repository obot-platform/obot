import { handleRouteError } from '$lib/errors';
import { AdminService, type Project } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let projects: Project[] = [];
	let users: OrgUser[] = [];
	try {
		projects = await AdminService.listProjects({ fetch });
		users = await AdminService.listUsers({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/v2/projects', profile.current);
	}

	return {
		projects,
		users
	};
};
