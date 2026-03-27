import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let messagePolicy;
	try {
		messagePolicy = await AdminService.getMessagePolicy(id, { fetch });
	} catch (err) {
		handleRouteError(err, `/admin/message-policies/${id}`, profile.current);
	}

	return {
		messagePolicy
	};
};
