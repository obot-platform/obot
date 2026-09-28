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
		handleRouteError(err, `/models/message-policies/${id}`, profile.current);
	}

	if (messagePolicy && messagePolicy.direction !== 'user-message') {
		throw redirect(
			302,
			messagePolicy.direction === 'tool-calls'
				? `/mcp-servers/message-policies/${id}`
				: `/admin/message-policies/${id}`
		);
	}

	return {
		messagePolicy
	};
};
