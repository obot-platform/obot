import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import { profile, version } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	if (!version.current.authEnabled) {
		throw redirect(302, '/admin');
	}

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
