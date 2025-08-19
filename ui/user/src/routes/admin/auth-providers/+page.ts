import { handleRouteError } from '$lib/errors';
import { AdminService, ChatService } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.authEnabled) {
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
