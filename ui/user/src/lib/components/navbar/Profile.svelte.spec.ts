import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import Profile from './Profile.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function openMenu(bootstrap = false) {
	const profile = { ...createMockProfile([Group.USER]), isBootstrapUser: () => bootstrap };
	await preparePageData({ profile });
	render(Profile);
	await page.getByRole('button').first().click();
}

describe('profile menu', () => {
	it('links to the agents acting for the user', async () => {
		await openMenu();

		await expect
			.element(page.getByRole('link', { name: 'Agents acting for you' }))
			.toHaveAttribute('href', '/agents/delegations');
	});

	it('has no agents acting for the bootstrap login', async () => {
		await openMenu(true);

		await expect.element(page.getByRole('button', { name: 'Log out' })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Agents acting for you' }))
			.not.toBeInTheDocument();
	});
});
