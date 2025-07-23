import { ChatService, getProfile, type AuthProvider } from '$lib/services';
import { Role } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url }) => {
	let authProviders: AuthProvider[] = [];
	let profile;

	try {
		profile = await getProfile({ fetch });
	} catch (_err) {
		authProviders = await ChatService.listAuthProviders({ fetch });
	}

	const loggedIn = profile?.loaded ?? false;
	const isAdmin = profile?.role === Role.ADMIN;

	if (loggedIn) {
		const redirectRoute = url.searchParams.get('rd');
		if (redirectRoute) {
			throw redirect(302, redirectRoute);
		}

		// Redirect to appropriate dashboard
		throw redirect(302, isAdmin ? '/v2/admin/mcp-servers' : '/mcp-servers');
	} else if (authProviders.length === 0) {
		// If no auth providers are configured, redirect to admin page for bootstrap login
		throw redirect(302, '/v2/admin');
	}

	return {
		loggedIn,
		authProviders
	};
};
