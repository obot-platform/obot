import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AgentSource, Harness, HostedAgent } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hostedAgents: HostedAgent[] = [];
	let agentSources: AgentSource[] = [];
	let harnesses: Harness[] = [];

	try {
		// Admins get the unfiltered list; the default view is access-rule filtered.
		[hostedAgents, agentSources, harnesses] = await Promise.all([
			AdminService.listHostedAgents({ fetch, all: true }),
			AdminService.listAgentSources({ fetch }),
			AdminService.listHarnesses({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, '/admin/hosted-agents', profile);
	}

	return {
		hostedAgents,
		agentSources,
		harnesses
	};
};
