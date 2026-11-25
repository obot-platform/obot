import * as navigation from '$lib/navigation';
import type { AuthProvider, Profile } from '$lib/services';
import type { PageProps } from './$types';
import HomePage from './+page.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

const oktaProvider = {
	id: 'okta-auth-provider',
	namespace: 'default',
	name: 'Okta',
	icon: '/okta.svg'
} as AuthProvider;

function signedOutProfile(accountInactive: boolean, environmentRestricted = false): Profile {
	return {
		id: '',
		email: '',
		iconURL: '',
		role: 0,
		effectiveRole: 0,
		groups: [],
		unauthorized: true,
		accountInactive,
		environmentRestricted,
		username: ''
	};
}

function renderHomePage(accountInactive: boolean, environmentRestricted = false) {
	return render(HomePage, {
		data: {
			loggedIn: false,
			authProviders: [oktaProvider],
			profile: signedOutProfile(accountInactive, environmentRestricted)
		} as unknown as PageProps['data'],
		params: {}
	});
}

describe('home page', () => {
	it('tells a user whose account is not active why they were signed out', async () => {
		renderHomePage(true);

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'You are not authorized to access this application. Contact your administrator.'
			);
		await expect.element(page.getByRole('button', { name: /Continue with Okta/ })).toBeVisible();
	});

	it('tells a user signed out of a restricted environment why', async () => {
		renderHomePage(false, true);

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'Only admins can sign in to this environment right now. Contact an admin.'
			);
		await expect.element(page.getByRole('button', { name: /Continue with Okta/ })).toBeVisible();
	});

	it('does not show the notice to a signed-out visitor', async () => {
		renderHomePage(false);

		await expect.element(page.getByRole('button', { name: /Continue with Okta/ })).toBeVisible();
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	});
});

describe('sign-in with a login hint', () => {
	afterEach(() => {
		vi.mocked(navigation.navigateTo).mockRestore();
		window.history.replaceState(null, '', window.location.pathname);
	});

	it("passes an agent approval link's login hint on to the provider", async () => {
		vi.mocked(navigation.navigateTo).mockImplementation(() => {});
		window.history.replaceState(
			null,
			'',
			window.location.pathname +
				'?rd=' +
				encodeURIComponent('/agents/approve/adr1req') +
				'&login_hint=' +
				encodeURIComponent('ada@example.com')
		);
		renderHomePage(false);

		await page.getByRole('button', { name: /Continue with Okta/ }).click();

		expect(navigation.navigateTo).toHaveBeenCalledWith(
			'/oauth2/start?rd=%2Fagents%2Fapprove%2Fadr1req&obot-auth-provider=default/okta-auth-provider&login_hint=ada%40example.com'
		);
	});
});
