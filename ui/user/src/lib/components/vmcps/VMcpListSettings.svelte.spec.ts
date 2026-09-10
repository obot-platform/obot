import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import VMcpListSettings from './VMcpListSettings.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderSettings(groups: string[] = [Group.ADMIN]) {
	await preparePageData({
		profile: createMockProfile(groups)
	});
	return render(VMcpListSettings);
}

async function openFilters() {
	await page.getByRole('button', { name: 'Filters' }).click();
	await expect.element(page.getByRole('heading', { name: 'vMCPs Settings' })).toBeVisible();
}

describe('VMcpListSettings.svelte', () => {
	it('shows the shared vMCP filter for admins', async () => {
		await renderSettings([Group.ADMIN]);
		await openFilters();

		await expect
			.element(page.getByRole('checkbox', { name: 'Show shared vMCPs only' }))
			.toBeVisible();
		await expect.element(page.getByRole('checkbox', { name: 'Show my vMCPs only' })).toBeVisible();
	});

	it('hides the shared vMCP filter for users without admin access', async () => {
		await renderSettings([Group.USER]);
		await openFilters();

		await expect
			.element(page.getByRole('checkbox', { name: 'Show shared vMCPs only' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('checkbox', { name: 'Show my vMCPs only' })).toBeVisible();
	});

	it('clears the other ownership filter when one is checked', async () => {
		await renderSettings([Group.ADMIN]);
		await openFilters();

		const shared = page.getByRole('checkbox', { name: 'Show shared vMCPs only' });
		const mine = page.getByRole('checkbox', { name: 'Show my vMCPs only' });

		await shared.click();
		await expect.element(shared).toBeChecked();
		await expect.element(mine).not.toBeChecked();

		await mine.click();
		await expect.element(mine).toBeChecked();
		await expect.element(shared).not.toBeChecked();
	});
});
