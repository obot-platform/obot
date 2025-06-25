import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let authProviders: AuthProvider[] = [];
	try {
		authProviders = await ChatService.listAuthProviders({ fetch });
	} catch (err) {
		handleRouteError(err, '/v2/admin/auth-providers', profile.current);
	}

	return {
		authProviders
	};
};
