import { handleRouteError } from '$lib/errors';
import { AdminService, type HostedAgent } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	let hostedAgent: HostedAgent | undefined;
	try {
		hostedAgent = await AdminService.getHostedAgent(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/hosted-agents/${id}`, profile);
	}

	return {
		hostedAgent
	};
};
