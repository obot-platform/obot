import { BOOTSTRAP_USER_ID } from '$lib/constants';
import { ChatService, getProfile, type AuthProvider } from '$lib/services';
import { Role } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let authProviders: AuthProvider[] = [];
	let profile;

	try {
		profile = await getProfile({ fetch });
	} catch (_err) {
		authProviders = await ChatService.listAuthProviders({ fetch });
	}

	if (profile?.role === Role.ADMIN) {
		throw redirect(
			307,
			profile.username === BOOTSTRAP_USER_ID ? '/v2/admin/auth-providers' : '/v2/admin/mcp-servers'
		);
	}

	return {
		loggedIn: profile?.loaded ?? false,
		isAdmin: profile?.role === Role.ADMIN,
		authProviders
	};
};
