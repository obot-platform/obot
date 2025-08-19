import { BOOTSTRAP_USER_ID } from '$lib/constants';
import { ChatService, getProfile, type AuthProvider } from '$lib/services';
import { Role } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	let authProviders: AuthProvider[] = [];
	let profile;
	let version;

	try {
		version = await ChatService.getVersion({ fetch });
		profile = await getProfile({ fetch });
	} catch (_err) {
		authProviders = await ChatService.listAuthProviders({ fetch });
	}

	if (profile?.role === Role.ADMIN) {
		throw redirect(
			307,
			profile.username === BOOTSTRAP_USER_ID && version?.authEnabled
				? '/admin/auth-providers'
				: '/admin/mcp-servers'
		);
	}

	return {
		loggedIn: profile?.loaded ?? false,
		isAdmin: profile?.role === Role.ADMIN,
		authProviders
	};
};
