import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();

	try {
		const instance = await AdminService.getHostedAgentInstance(params.instance_id, { fetch });
		// The agent is what decides whether there is a terminal at all, so it is
		// fetched here rather than inferred from the instance.
		const agent = await AdminService.getHostedAgent(instance.hostedAgentID, { fetch });
		return { instance, agent };
	} catch (err) {
		handleRouteError(err, `/hosted-agents/${params.instance_id}/terminal`, profile);
	}
};
