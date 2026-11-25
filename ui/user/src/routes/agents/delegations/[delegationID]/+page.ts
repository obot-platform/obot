import { createHttpError, handleRouteError } from '$lib/errors';
import { AgentIdentitiesService } from '$lib/services';
import type { PageLoad } from './$types';

// The user's view of one agent acting for them. The list of their delegations is the only read of one;
// another user's delegation is not in it.
export const load: PageLoad = async ({ fetch, parent, params, url }) => {
	const { profile } = await parent();
	const path = `/agents/delegations/${params.delegationID}`;
	try {
		const delegations = await AgentIdentitiesService.listMyAgentDelegations({ fetch });
		const delegation = delegations.find((candidate) => candidate.id === params.delegationID);
		if (!delegation) {
			throw createHttpError(404, path, 'agent delegation not found');
		}
		return { delegation, oauthError: url.searchParams.get('oauth_error') };
	} catch (err) {
		handleRouteError(err, path, profile);
	}
};
