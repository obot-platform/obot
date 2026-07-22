import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { HostedAgent } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hostedAgents: HostedAgent[] = [];

	try {
		// Access-policy filtered: users only see the agents granted to them.
		hostedAgents = await AdminService.listHostedAgents({ fetch });
	} catch (err) {
		handleRouteError(err, '/hosted-agents', profile);
	}

	return {
		hostedAgents
	};
};
