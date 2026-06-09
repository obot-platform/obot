import { handleRouteError } from '$lib/errors';
import { UserService, type License } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let license: License | undefined = undefined;
	try {
		license = await UserService.getLicense({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/license', profile);
	}

	return {
		license
	};
};
