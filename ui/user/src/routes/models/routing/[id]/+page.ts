import { handleRouteError } from '$lib/errors';
import { getRoutingRule } from '$lib/services/llm-gateway/gateway';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	if (!profile?.hasAdminAccess?.()) {
		throw redirect(307, '/');
	}

	try {
		const rule = await getRoutingRule(params.id, { fetch });
		return { rule };
	} catch (err) {
		handleRouteError(err, `/models/routing/${params.id}`, profile);
	}
};
