import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let authProviders: AuthProvider[] = [];
	try {
		authProviders = await AdminService.listAuthProviders({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/auth-providers', profile.current);
	}

	return {
		authProviders
	};
};
