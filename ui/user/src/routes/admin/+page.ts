import { CommonAuthProviderIds } from '$lib/constants';
import {
	AdminService,
	UserService,
	getProfile,
	type AuthProvider,
	type BootstrapStatus,
	type Profile
} from '$lib/services';
import { Group } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

function getAdminRedirectPath(profile?: Profile, hasVMCPs?: boolean): string {
	if (profile?.isBootstrapUser?.()) {
		return '/identity-access?view=auth-providers';
	}

	if (profile?.isOwner?.() && !hasVMCPs) {
		return '/vmcps?new=true';
	}

	return '/dashboard';
}

export const load: PageLoad = async ({ fetch, url }) => {
	let authProviders: AuthProvider[] = [];
	let bootstrapStatus: BootstrapStatus | undefined;
	let profile;

	try {
		profile = await getProfile({ fetch });
	} catch (_err) {
		[bootstrapStatus, authProviders] = await Promise.all([
			UserService.getBootstrapStatus(),
			UserService.listAuthProviders({ fetch })
		]);
	}

	const showSetupHandoff = url.searchParams.get('setup') === 'complete';
	const hasAccess =
		profile?.groups.includes(Group.ADMIN) || profile?.groups.includes(Group.AUDITOR);
	if (hasAccess && !showSetupHandoff) {
		const vmcps = await AdminService.listAllVMCPs({ fetch });
		throw redirect(307, getAdminRedirectPath(profile, vmcps.length > 0));
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
