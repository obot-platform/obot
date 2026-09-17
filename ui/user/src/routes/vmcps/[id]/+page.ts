import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import { version } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, params, fetch }) => {
	const { profile } = await parent();

	try {
		const [users, vmcp, latestVersion] = await Promise.all([
			UserService.listUsersIncludeDeleted({ fetch }),
			UserService.getVMCP(params.id, { fetch }),
			UserService.getVersion({ fetch, dontLogErrors: true }).catch(() => undefined)
		]);
		if (latestVersion) {
			version.initialize(latestVersion);
		}
		return { vmcp, users };
	} catch (err) {
		handleRouteError(err, `/vmcps/${params.id}`, profile);
	}
};
