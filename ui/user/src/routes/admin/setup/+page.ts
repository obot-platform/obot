import { CommonAuthProviderIds, SEEN_SPLASH_DIALOG_KEY } from '$lib/constants';
import { handleRouteError } from '$lib/errors';
import { hasSeenTimestamp } from '$lib/localstate';
import { AdminService, UserService, type AuthProvider, type LocalAuthUser } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	const bootstrapStatus = await UserService.getBootstrapStatus();

	if (!profile.isBootstrapUser?.() || !bootstrapStatus?.setupEnabled) {
		throw redirect(307, '/dashboard');
	}

	let authProviders: AuthProvider[] = [];
	try {
		authProviders = await AdminService.listAuthProviders({ fetch });
	} catch (err) {
		handleRouteError(err, url.pathname, profile);
	}

	const localProvider = authProviders.find(
		(provider) => provider.id === CommonAuthProviderIds.LOCAL
	);
	const otherConfigured = authProviders.some(
		(provider) => provider.configured && provider.id !== CommonAuthProviderIds.LOCAL
	);
	if (otherConfigured && !localProvider?.configured) {
		throw redirect(307, '/identity-access?view=auth-providers');
	}

	// getting local users in case of subsequent revisit to setup
	// ex. bootstrap leaves and comes back
	let localUsers: LocalAuthUser[] = [];
	if (localProvider?.configured) {
		try {
			localUsers = await AdminService.listLocalAuthUsers({ fetch });
		} catch (err) {
			handleRouteError(err, url.pathname, profile);
		}
	}

	let eulaAccepted = false;
	try {
		eulaAccepted = (await AdminService.getEula({ fetch })).accepted;
	} catch {
		// ignore error; default to false
	}

	return {
		localProvider,
		localUsers,
		eulaAccepted,
		splashSeen: hasSeenTimestamp(SEEN_SPLASH_DIALOG_KEY, profile.created)
	};
};
