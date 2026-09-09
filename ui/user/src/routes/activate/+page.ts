import { browser } from '$app/environment';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	if (profile.requirePasswordChange) {
		return;
	}

	const authProviders = await UserService.listAuthProviders({ fetch });
	if (profile.loaded || !authProviders.some((provider) => provider.requiresActivation)) {
		// Nothing to activate. Drop any fragment from history before leaving.
		if (browser) {
			window.history.replaceState(null, '', window.location.pathname + window.location.search);
		}
		throw redirect(302, '/');
	}
};
