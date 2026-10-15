import { UNAUTHORIZED_PATHS } from '$lib/constants';
import OktaSCIMPage from '../okta-scim/+page.svelte';
import SCIMProvisioningPage from './+page.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('SCIM provisioning app page', () => {
	it.each([
		['/scim-provisioning', SCIMProvisioningPage],
		// The page's first address, which existing Okta SCIM apps still use.
		['/okta-scim', OktaSCIMPage]
	])('at %s explains that the app does not sign in, and links to Obot', async (_, Page) => {
		render(Page);

		await expect
			.element(page.getByRole('heading', { name: 'This app is for provisioning only' }))
			.toBeVisible();
		await expect
			.element(page.getByText('Your identity provider uses this app', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByText('It does not sign you in.', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Go to Obot sign-in' }))
			.toHaveAttribute('href', '/');
	});

	it('is shown without an Obot session', () => {
		// People open it from their identity provider, usually before signing in to Obot, so a refused
		// request for their profile must not redirect them away.
		expect(UNAUTHORIZED_PATHS.has('/scim-provisioning')).toBe(true);
		expect(UNAUTHORIZED_PATHS.has('/okta-scim')).toBe(true);
	});
});
