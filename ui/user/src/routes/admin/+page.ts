import { UserService, getProfile, type AuthProvider } from '$lib/services';
import { Group } from '$lib/services/admin/types';
import type { Profile } from '$lib/services/user/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

const NEW_USER_REDIRECT_WINDOW_MS = 10 * 60 * 1000;

function getAdminRedirectPath(profile?: Profile): string {
	if (profile?.isBootstrapUser?.()) {
		return '/identity-access?view=auth-providers';
	}

	const created = profile?.created ? new Date(profile.created) : null;
	if (created && Date.now() - created.getTime() < NEW_USER_REDIRECT_WINDOW_MS) {
		return '/vmcps';
	}

	return '/dashboard';
}

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
		throw redirect(307, getAdminRedirectPath(profile));
	}

	return {
		loggedIn: profile?.loaded ?? false,
		hasAccess,
		authProviders,
		showSetupHandoff
	};
};
