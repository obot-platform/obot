import { handleRouteError } from '$lib/errors';
import { AgentIdentitiesService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	try {
		return { delegations: await AgentIdentitiesService.listMyAgentDelegations({ fetch }) };
	} catch (err) {
		handleRouteError(err, '/agents/delegations', profile);
	}
};
