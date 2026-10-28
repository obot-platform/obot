import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const { id } = params;

	let modelAccessPolicy: ModelAccessPolicy | undefined;
	if (id !== 'new') {
		try {
			modelAccessPolicy = await AdminService.getModelAccessPolicy(id, { fetch });
		} catch (err) {
			handleRouteError(err, `/models/profiles/${id}`, profile.current);
		}
	}

	return {
		modelAccessPolicy
	};
};
