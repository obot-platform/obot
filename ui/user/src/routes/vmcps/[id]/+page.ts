import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, params, fetch }) => {
	const { profile } = await parent();

	try {
		const users = await UserService.listUsersIncludeDeleted({ fetch });
		const vmcp = await UserService.getVMCP(params.id, { fetch });
		return { vmcp, users };
	} catch (err) {
		handleRouteError(err, `/vmcps/${params.id}`, profile);
	}
};
