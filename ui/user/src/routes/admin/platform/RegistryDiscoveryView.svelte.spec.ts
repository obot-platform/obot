import { Group } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import RegistryDiscoveryView from './RegistryDiscoveryView.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Registry discovery settings', () => {
	it('locks the toggle during a Platform save while keeping the risk notice visible', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		await render(RegistryDiscoveryView, {
			settings: { anonymousAccessEnabled: true },
			saving: true
		});
		const toggle = page.getByRole('checkbox', { name: 'Allow anonymous registry discovery' });
		await expect.element(toggle).toBeChecked();
		await expect.element(toggle).toBeDisabled();
		await expect
			.element(page.getByText('Anonymous discovery publishes server metadata'))
			.toBeVisible();
	});
});
