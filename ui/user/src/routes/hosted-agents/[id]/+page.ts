import { handleRouteError } from '$lib/errors';
import { AdminService, type HostedAgent, type HostedAgentInstance } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	let hostedAgent: HostedAgent | undefined;
	let instances: HostedAgentInstance[] = [];
	try {
		hostedAgent = await AdminService.getHostedAgent(id, { fetch });
		instances = await AdminService.listHostedAgentInstances(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/hosted-agents/${id}`, profile);
	}

	return {
		hostedAgent,
		instances
	};
};
