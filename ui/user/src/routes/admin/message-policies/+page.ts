import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { MessagePolicy } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
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
