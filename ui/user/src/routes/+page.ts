import { safeRedirectPath } from '$lib/redirect';
import { UserService, type AuthProvider, type BootstrapStatus, Group } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, url, parent }) => {
	const { profile } = await parent();
	const loggedIn = profile?.loaded ?? false;
	const appBasePath = url.pathname === '/obot' || url.pathname.startsWith('/obot/') ? '/obot' : '';

	let bootstrapStatus: BootstrapStatus | undefined;
	let authProviders: AuthProvider[] = [];
	if (!loggedIn) {
		[bootstrapStatus, authProviders] = await Promise.all([
			UserService.getBootstrapStatus(),
			UserService.listAuthProviders({ fetch })
		]);
	}
	const isAdminOrOwner =
		profile?.groups.includes(Group.ADMIN) || profile?.groups.includes(Group.OWNER);

	if (loggedIn) {
		const redirectRoute = safeRedirectPath(url.searchParams.get('rd'), appBasePath);
		if (redirectRoute) {
			throw redirect(302, redirectRoute);
		}

		const defaultRoute = isAdminOrOwner ? '/admin/dashboard' : '/mcp-servers';
		if (appBasePath) {
			throw redirect(302, `${appBasePath}${defaultRoute}`);
		}
		throw redirect(302, defaultRoute);
	}

	if (bootstrapStatus?.enabled && authProviders.length === 0) {
		// If no auth providers are available, redirect to the admin page for bootstrap login.
		if (appBasePath) {
			throw redirect(302, `${appBasePath}/admin`);
		}
		throw redirect(302, '/admin');
	}

	return {
		loggedIn,
		authProviders
	};
};
