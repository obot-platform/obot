import { handleRouteError } from '$lib/errors';
import { AdminService, UserService } from '$lib/services';
import type { AuthProvider, License } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	const version = await UserService.getVersion({ fetch });
	if (!version.authEnabled) {
		throw redirect(302, '/admin');
	}

	let authProviders: AuthProvider[] = [];
	let license: License | undefined = undefined;
	try {
		authProviders = await AdminService.listAuthProviders({ fetch });
		license = await AdminService.getLicense({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/auth-providers', profile.current);
	}

	return {
		authProviders,
		license
	};
};
