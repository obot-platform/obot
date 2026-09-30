import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch }) => {
	const version = await UserService.getVersion({ fetch });
	if (!version.messagePoliciesEnabled) {
		throw redirect(302, '/models');
	}

	const { id } = params;

	let messagePolicy;
	try {
		messagePolicy = await AdminService.getMessagePolicy(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/models/ai-judge-policies/${id}`, profile.current);
	}

	if (messagePolicy && messagePolicy.direction === 'tool-calls') {
		throw redirect(302, `/mcp-servers/ai-judge-policies/${id}`);
	}

	return {
		messagePolicy
	};
};
