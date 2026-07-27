import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	try {
		return {
			gitCredentials: await AdminService.listGitCredentials({ fetch, dontLogErrors: true })
		};
	} catch (err) {
		handleRouteError(err, '/admin/git-credentials', profile);
		return { gitCredentials: [] };
	}
};
