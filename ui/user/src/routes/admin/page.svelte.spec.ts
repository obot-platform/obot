import * as navigation from '$lib/navigation';
import { Group } from '$lib/services';
import type { AuthProvider, TempUser } from '$lib/services/admin/types';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import AdminPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

afterEach(() => {
	vi.mocked(navigation.navigateTo).mockRestore();
});

const tempUser: TempUser = {
	userId: 2,
	username: '00u-owner',
	email: 'owner@example.com',
	role: 0,
	groups: [],
	iconUrl: '',
	authProviderName: 'okta-auth-provider',
	authProviderNamespace: 'default',
	cachedAt: '2026-09-29T00:00:00.000Z'
};

function okta(overrides: Partial<AuthProvider> = {}): AuthProvider {
	return {
		id: 'okta-auth-provider',
		created: '2026-09-01T00:00:00.000Z',
		type: 'authprovider',
		name: 'Okta',
		image: '',
		port: 0,
		configured: true,
		namespace: 'default',
		...overrides
	};
}

// Renders the Owner Setup handoff, where the bootstrap user confirms the account that signed in.
async function renderSetupHandoff(authProvider: AuthProvider) {
	const confirmOwner = vi.fn();
	worker.use(
		http.get('/api/setup/temp-user', () => HttpResponse.json(tempUser)),
		http.get('/api/setup/explicit-role-emails', () =>
			HttpResponse.json({ owners: null, admins: null })
		),
		http.get('/api/auth-providers', () => HttpResponse.json({ items: [authProvider] })),
		http.post('/api/setup/confirm-owner', () => {
			confirmOwner();
			return HttpResponse.json({ success: true, userId: 2, email: tempUser.email, message: '' });
		})
	);

	const profile = createMockProfile([Group.OWNER, Group.ADMIN]);
	profile.username = 'bootstrap';
	profile.isBootstrapUser = () => true;
	const data = await preparePageData<PageData>({
		profile,
		authProviders: [],
		loggedIn: true,
		hasAccess: true,
		showSetupHandoff: true
	});
	render(AdminPage, { data });
	worker.use(http.post('/api/bootstrap/logout', () => new HttpResponse(null, { status: 204 })));

	await page.getByRole('button', { name: 'Yes, make this account an owner' }).click();
	await vi.waitFor(() => expect(confirmOwner).toHaveBeenCalledOnce());
	await expect.element(page.getByText(/You've established your first owner user/)).toBeVisible();
}

// Logs out, and returns where signing out sends the browser, and where the browser ends up: that page, or, when it
// is the sign-in page, the page that signing in returns it to. A page that needs a session keeps only its path when it
// sends a signed-out browser to sign in, so a view of a page is kept only when signing out sends the browser straight
// to the sign-in page.
async function logOut() {
	vi.mocked(navigation.navigateTo).mockImplementation(() => {});
	await page.getByRole('button', { name: 'Log out' }).click();
	await vi.waitFor(() => expect(navigation.navigateTo).toHaveBeenCalledOnce());
	const signOut = new URL(vi.mocked(navigation.navigateTo).mock.calls[0][0], 'http://localhost');
	const afterSignOut = new URL(signOut.searchParams.get('rd') ?? '', 'http://localhost');
	const destination =
		afterSignOut.pathname === '/'
			? (afterSignOut.searchParams.get('rd') ?? '/')
			: afterSignOut.pathname;
	return { afterSignOut: afterSignOut.pathname, destination };
}

describe('Owner Setup handoff', () => {
	it('continues on the SCIM tab for a provider that provisions through SCIM', async () => {
		await renderSetupHandoff(okta({ scimState: 'connected' }));

		await expect
			.element(page.getByText(/Okta provisions users and groups through SCIM/))
			.toBeVisible();
		await expect.element(page.getByText(/Identity & Access → Auth Providers → SCIM/)).toBeVisible();
		expect(await logOut()).toEqual({
			afterSignOut: '/',
			destination: '/identity-access?view=auth-providers&subview=scim'
		});
	});

	it('says nothing about SCIM once it is enforced', async () => {
		await renderSetupHandoff(okta({ scimState: 'enforced' }));

		await expect
			.element(page.getByText(/provisions users and groups through SCIM/))
			.not.toBeInTheDocument();
		expect(await logOut()).toEqual({
			afterSignOut: '/admin',
			destination: '/admin'
		});
	});

	it('says nothing about SCIM for a provider that synchronizes its directory', async () => {
		await renderSetupHandoff(okta());

		await expect
			.element(page.getByText(/provisions users and groups through SCIM/))
			.not.toBeInTheDocument();
		expect(await logOut()).toEqual({
			afterSignOut: '/admin',
			destination: '/admin'
		});
	});
});
