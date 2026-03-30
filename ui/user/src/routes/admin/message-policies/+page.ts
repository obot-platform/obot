import { handleRouteError } from '$lib/errors';
import { AdminService, ChatService } from '$lib/services';
import type { MessagePolicy } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.messagePoliciesEnabled) {
		throw redirect(302, '/admin');
	}

	let messagePolicies: MessagePolicy[] = [];

	try {
		messagePolicies = await AdminService.listMessagePolicies({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/message-policies', profile.current);
	}

	return {
		messagePolicies
	};
};
