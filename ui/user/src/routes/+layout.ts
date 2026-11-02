import { dev } from '$app/environment';
import { getHttpStatusCode } from '$lib/errors';
import {
	UserService,
	type AppNotification,
	type AppPreferences,
	type Profile,
	type Version
} from '$lib/services';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import { isBasicUser } from '$lib/utils';
import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const prerender = 'auto';
export const ssr = dev;

const ACCOUNT_INACTIVE_KEY = 'obot-account-inactive';
// ACCOUNT_INACTIVE_COOKIE is set by the server as it sends a refused page load of an account that is not active to the
// login page (accountInactiveCookie in pkg/api/server/server.go). A link can add a parameter to the login page's URL,
// but cannot set a cookie, so only the cookie is trusted.
const ACCOUNT_INACTIVE_COOKIE = 'obot_account_inactive';
const BASIC_USER_ALLOWED_ROUTE_PREFIXES = [
	'/market',
	'/login',
	'/auth',
	'/logout',
	'/change-password',
	'/activate',
	'/agent-auth-scopes',
	'/install-cli',
	'/oauth-debugger',
	'/privacy-policy'
];

// takeAccountInactiveCookie reports whether the server set ACCOUNT_INACTIVE_COOKIE, and clears it, so that it is read
// once.
function takeAccountInactiveCookie(): boolean {
	if (typeof document === 'undefined') {
		return false;
	}
	const set = document.cookie
		.split(';')
		.some((cookie) => cookie.trim() === `${ACCOUNT_INACTIVE_COOKIE}=true`);
	if (set) {
		document.cookie = `${ACCOUNT_INACTIVE_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
	}
	return set;
}

// accountInactive reports whether to tell the visitor that their account is not active. The server refuses every
// request from such an account and ends its session, so only the first response says why: a 403 from the API, or a
// redirect of a page load to the login page with ACCOUNT_INACTIVE_COOKIE. The answer is kept for the rest of the visit,
// so that it survives the redirects that follow, until someone signs in.
function accountInactive(profileResult: PromiseSettledResult<Profile>): boolean {
	const redirected = takeAccountInactiveCookie();
	const refused =
		profileResult.status === 'rejected' &&
		(getHttpStatusCode(profileResult.reason) === 403 || redirected);
	if (typeof sessionStorage === 'undefined') {
		return refused;
	}
	if (refused) {
		sessionStorage.setItem(ACCOUNT_INACTIVE_KEY, 'true');
		return true;
	}
	if (profileResult.status === 'fulfilled') {
		sessionStorage.removeItem(ACCOUNT_INACTIVE_KEY);
		return false;
	}
	return sessionStorage.getItem(ACCOUNT_INACTIVE_KEY) === 'true';
}

export const load: LayoutLoad = async ({ fetch, url }) => {
	const [versionResult, appPreferencesResult, profileResult] = await Promise.allSettled([
		UserService.getVersion({ fetch }),
		UserService.listAppPreferences({ fetch }),
		UserService.getProfile({ fetch })
	]);

	const version: Version | undefined =
		versionResult.status === 'fulfilled' ? versionResult.value : undefined;
	const appPreferences: AppPreferences =
		appPreferencesResult.status === 'fulfilled'
			? compileAppPreferences(appPreferencesResult.value)
			: compileAppPreferences();
	const inactive = accountInactive(profileResult);
	const profile: Profile =
		profileResult.status === 'fulfilled'
			? profileResult.value
			: {
					id: '',
					email: '',
					iconURL: '',
					role: 0,
					effectiveRole: 0,
					groups: [],
					unauthorized: true,
					accountInactive: inactive,
					username: ''
				};

	if (
		profile.requirePasswordChange &&
		url.pathname !== '/change-password' &&
		url.pathname !== '/activate'
	) {
		throw redirect(303, `/change-password?rd=${encodeURIComponent(url.pathname + url.search)}`);
	}

	if (
		profile.loaded &&
		isBasicUser(profile.groups) &&
		!BASIC_USER_ALLOWED_ROUTE_PREFIXES.some((prefix) => url.pathname.startsWith(prefix))
	) {
		throw redirect(302, '/market');
	}

	let appNotification: AppNotification | undefined;
	// A restricted session is refused this request, and the password page renders without it,
	// so avoid a guaranteed 403 on every load.
	if (!profile.unauthorized && !profile.requirePasswordChange) {
		const [appNotificationResult] = await Promise.allSettled([
			UserService.getAppNotification({ fetch })
		]);
		appNotification =
			appNotificationResult.status === 'fulfilled' ? appNotificationResult.value : undefined;
	}

	return {
		appPreferences,
		profile,
		version,
		appNotification
	};
};
