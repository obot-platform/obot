import type { AuthProvider, Profile } from '$lib/services';
import type { PageProps } from './$types';
import HomePage from './+page.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const oktaProvider = {
	id: 'okta-auth-provider',
	namespace: 'default',
	name: 'Okta',
	icon: '/okta.svg'
} as AuthProvider;

function signedOutProfile(accountInactive: boolean): Profile {
	return {
		id: '',
		email: '',
		iconURL: '',
		role: 0,
		effectiveRole: 0,
		groups: [],
		unauthorized: true,
		accountInactive,
		username: ''
	};
}

function renderHomePage(accountInactive: boolean) {
	return render(HomePage, {
		data: {
			loggedIn: false,
			authProviders: [oktaProvider],
			profile: signedOutProfile(accountInactive)
		} as unknown as PageProps['data'],
		params: {}
	});
}

describe('home page', () => {
	it('tells a user whose account is not active why they were signed out', async () => {
		renderHomePage(true);

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('Your account is not active. Contact your administrator.');
		await expect.element(page.getByRole('button', { name: /Continue with Okta/ })).toBeVisible();
	});

	it('does not show the notice to a signed-out visitor', async () => {
		renderHomePage(false);

		await expect.element(page.getByRole('button', { name: /Continue with Okta/ })).toBeVisible();
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	});
});
