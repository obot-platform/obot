import { UserService, getProfile, type AuthProvider } from '$lib/services';
import { Group } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url }) => {
	let authProviders: AuthProvider[] = [];
	let profile;

	try {
		profile = await getProfile({ fetch });
	} catch (_err) {
		authProviders = await UserService.listAuthProviders({ fetch });
	}

	const showSetupHandoff = url.searchParams.get('setup') === 'complete';
	const hasAccess =
		profile?.groups.includes(Group.ADMIN) || profile?.groups.includes(Group.AUDITOR);
	if (hasAccess && !showSetupHandoff) {
		throw redirect(
			307,
			profile?.isBootstrapUser?.() ? '/admin/auth-providers' : '/admin/dashboard'
		);
	}

	return {
		loggedIn: profile?.loaded ?? false,
		hasAccess,
		authProviders,
		showSetupHandoff
	};
};
