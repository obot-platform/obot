import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type {
	HostedAgent,
	HostedAgentPool,
	HostedAgentPoolAssignment,
	HostedAgentInstance
} from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hostedAgents: HostedAgent[] = [];
	let instances: HostedAgentInstance[] = [];
	let pools: HostedAgentPool[] = [];
	let assignments: HostedAgentPoolAssignment[] = [];

	try {
		// Access-policy filtered: users only see the agents granted to them.
		[hostedAgents, instances, pools, assignments] = await Promise.all([
			AdminService.listHostedAgents({ fetch }),
			AdminService.listHostedAgentInstances(undefined, { fetch }),
			AdminService.listHostedAgentPools({ fetch }),
			AdminService.listHostedAgentPoolAssignments({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, '/hosted-agents', profile);
	}

	return {
		hostedAgents,
		instances,
		pools,
		assignments
	};
};
