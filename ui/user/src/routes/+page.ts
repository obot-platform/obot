import { CommonAuthProviderIds } from '$lib/constants';
import { Group, UserService, type AuthProvider } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url, parent }) => {
	const { profile } = await parent();
	const loggedIn = profile?.loaded ?? false;

	let authProviders: AuthProvider[] = [];
	if (!loggedIn) {
		authProviders = await UserService.listAuthProviders({ fetch });
	}

	const bootstrapStatus = await UserService.getBootstrapStatus();

	if (loggedIn) {
		if (profile?.isBootstrapUser?.() && bootstrapStatus?.setupEnabled) {
			throw redirect(302, '/admin/setup');
		}

		const redirectRoute = url.searchParams.get('rd');
		if (redirectRoute) {
			throw redirect(302, redirectRoute);
		}

		const isAtLeastPoweruser =
			profile?.groups.includes(Group.POWERUSER) || profile?.hasAdminAccess?.();
		throw redirect(302, isAtLeastPoweruser ? '/dashboard' : '/vmcps');
	}

	if (bootstrapStatus?.enabled && authProviders.length === 0) {
		// If no auth providers are available, redirect to the admin page for bootstrap login.
		throw redirect(302, '/admin');
	}

	if (authProviders.length === 1 && authProviders[0].id === CommonAuthProviderIds.LOCAL) {
		const rd = url.searchParams.get('rd') || url.pathname;
		throw redirect(302, '/login/local?rd=' + encodeURIComponent(rd));
	}

	return {
		loggedIn,
		authProviders
	};
};
