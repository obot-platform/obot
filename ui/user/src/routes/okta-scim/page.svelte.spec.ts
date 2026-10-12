import { UNAUTHORIZED_PATHS } from '$lib/constants';
import OktaSCIMPage from './+page.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Okta SCIM provisioning app page', () => {
	it('explains that the app does not sign in, and links to Obot', async () => {
		render(OktaSCIMPage);

		await expect
			.element(page.getByRole('heading', { name: 'This app is for provisioning only' }))
			.toBeVisible();
		await expect
			.element(page.getByText('It does not sign you in.', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Go to Obot sign-in' }))
			.toHaveAttribute('href', '/');
	});

	it('is shown without an Obot session', () => {
		// People open it from Okta, usually before signing in to Obot, so a refused request for their
		// profile must not redirect them away.
		expect(UNAUTHORIZED_PATHS.has('/okta-scim')).toBe(true);
	});
});
