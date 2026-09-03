import { handleRouteError } from '$lib/errors';
import { AdminService, type HostedAgent } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { id } = params;
	const { profile } = await parent();

	if (!profile.hasAdminAccess?.()) {
		throw redirect(307, `/hosted-agents`);
	}

	let hostedAgent: HostedAgent | undefined;
	try {
		hostedAgent = await AdminService.getHostedAgent(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/hosted-agents/${id}`, profile);
	}

	return {
		hostedAgent
	};
};
