import { createHttpError, handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, params, fetch }) => {
	const { profile } = await parent();
	const path = `/vmcps/${params.id}/instance/${params.instanceid}`;

	try {
		const [vmcp, instance, users] = await Promise.all([
			UserService.getVMCP(params.id, { fetch }),
			UserService.getVMCPInstance(params.instanceid, { fetch }),
			UserService.listUsersIncludeDeleted({ fetch })
		]);
		if (instance.vmcpID !== vmcp.id) {
			handleRouteError(createHttpError(404, path, 'VMCP instance not found'), path, profile);
		}
		return { vmcp, instance, users };
	} catch (err) {
		handleRouteError(err, path, profile);
	}
};
