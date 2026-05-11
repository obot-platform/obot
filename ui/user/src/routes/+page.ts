import { AdminService, ChatService, type AuthProvider } from '$lib/services';
import { Group, type BootstrapStatus } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url, parent }) => {
	const { profile } = await parent();
	const loggedIn = profile?.loaded ?? false;

	let bootstrapStatus: BootstrapStatus | undefined;
	let authProviders: AuthProvider[] = [];
	if (!loggedIn) {
		[bootstrapStatus, authProviders] = await Promise.all([
			AdminService.getBootstrapStatus(),
			ChatService.listAuthProviders({ fetch })
		]);
	}
	const isAdminOrOwner =
		profile?.groups.includes(Group.ADMIN) || profile?.groups.includes(Group.OWNER);

	if (loggedIn) {
		const redirectRoute = url.searchParams.get('rd');
		if (redirectRoute) {
			throw redirect(302, redirectRoute);
		}

		const defaultRoute = isAdminOrOwner ? '/admin/dashboard' : '/mcp-servers';
		throw redirect(302, defaultRoute);
	}

	if (bootstrapStatus?.enabled && authProviders.length === 0) {
		// If no auth providers are configured, redirect to admin page for bootstrap login
		throw redirect(302, '/admin');
	}

	return {
		loggedIn,
		authProviders
	};
};
