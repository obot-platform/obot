import { UserService, type AuthProvider, type BootstrapStatus } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url, parent }) => {
	const { profile } = await parent();
	const loggedIn = profile?.loaded ?? false;

	let bootstrapStatus: BootstrapStatus | undefined;
	let authProviders: AuthProvider[] = [];
	if (!loggedIn) {
		[bootstrapStatus, authProviders] = await Promise.all([
			UserService.getBootstrapStatus(),
			UserService.listAuthProviders({ fetch })
		]);
	}

	if (loggedIn) {
		const redirectRoute = url.searchParams.get('rd');
		if (redirectRoute) {
			throw redirect(302, redirectRoute);
		}

		// change to /vmcps when implemented for both
		throw redirect(302, '/dashboard');
	}

	if (bootstrapStatus?.enabled && authProviders.length === 0) {
		// If no auth providers are available, redirect to the admin page for bootstrap login.
		throw redirect(302, '/admin');
	}

	return {
		loggedIn,
		authProviders
	};
};
