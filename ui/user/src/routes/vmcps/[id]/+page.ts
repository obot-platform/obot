import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent, params, fetch }) => {
	const { profile } = await parent();
	if (!profile.isAdmin?.()) {
		throw redirect(307, '/');
	}

	try {
		const vmcp = await UserService.getVMCP(params.id, { fetch });
		return { vmcp };
	} catch (err) {
		handleRouteError(err, `/vmcps/${params.id}`, profile);
	}
};
