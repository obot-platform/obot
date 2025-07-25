import { handleRouteError } from '$lib/errors';
import { AdminService, type BaseAgent } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let baseAgent: BaseAgent | undefined;
	try {
		baseAgent = await AdminService.getDefaultBaseAgent({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/chat-configuration', profile.current);
	}

	return {
		baseAgent
	};
};
