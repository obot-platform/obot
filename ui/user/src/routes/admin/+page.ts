import { CommonAuthProviderIds } from '$lib/constants';
import { AdminService, UserService, type AuthProvider, type Profile } from '$lib/services';
import { Group } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

function getAdminRedirectPath(
	profile?: Profile,
	hasVMCPs?: boolean,
	isSetupEnabled?: boolean
): string {
	if (profile?.isBootstrapUser?.() && isSetupEnabled) {
		return '/admin/setup';
	}

	if (profile?.isOwner?.() && !hasVMCPs) {
		return '/vmcps?new=true';
	}

	const isAtLeastPoweruser =
		profile?.groups.includes(Group.POWERUSER) || profile?.hasAdminAccess?.();
	return isAtLeastPoweruser ? '/dashboard' : '/vmcps';
}

export const load: PageLoad = async ({ fetch, url }) => {
	let authProviders: AuthProvider[] = [];
	let profile;

	try {
		profile = await UserService.getProfile({ fetch });
	} catch (_err) {
		authProviders = await UserService.listAuthProviders({ fetch });
	}

	const bootstrapStatus = await UserService.getBootstrapStatus();
	const showSetupHandoff = url.searchParams.get('setup') === 'complete';
	const hasAccess =
		profile?.groups.includes(Group.ADMIN) || profile?.groups.includes(Group.AUDITOR);
	if (hasAccess && !showSetupHandoff) {
		const vmcps = await AdminService.listAllVMCPs({ fetch });
		throw redirect(
			307,
			getAdminRedirectPath(profile, vmcps.length > 0, bootstrapStatus?.setupEnabled)
		);
	}

	if (
		!bootstrapStatus?.enabled &&
		!showSetupHandoff &&
		authProviders.length === 1 &&
		authProviders[0].id === CommonAuthProviderIds.LOCAL
	) {
		throw redirect(307, '/login/local?rd=' + encodeURIComponent(url.pathname));
	}

	return {
		loggedIn: profile?.loaded ?? false,
		hasAccess,
		authProviders,
		showSetupHandoff
	};
};
