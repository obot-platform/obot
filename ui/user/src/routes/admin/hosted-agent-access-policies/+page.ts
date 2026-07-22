import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { HostedAgentAccessPolicy } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hostedAgentAccessPolicies: HostedAgentAccessPolicy[] = [];

	try {
		hostedAgentAccessPolicies = await AdminService.listHostedAgentAccessPolicies({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/hosted-agent-access-policies', profile);
	}

	return {
		hostedAgentAccessPolicies
	};
};
