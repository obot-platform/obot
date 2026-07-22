import { handleRouteError } from '$lib/errors';
import { AdminService, type HostedAgentAccessPolicy } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	let hostedAgentAccessPolicy: HostedAgentAccessPolicy | undefined;
	try {
		hostedAgentAccessPolicy = await AdminService.getHostedAgentAccessPolicy(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/hosted-agent-access-policies/${id}`, profile);
	}

	return {
		hostedAgentAccessPolicy
	};
};
