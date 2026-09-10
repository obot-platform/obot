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
			.element(page.getByRole('checkbox', { name: 'Show only shared vMCPs' }))
			.toBeVisible();
		await expect.element(page.getByRole('checkbox', { name: 'Show only my vMCPs' })).toBeVisible();
	});

	it('hides the shared vMCP filter for users without admin access', async () => {
		await renderSettings([Group.USER]);
		await openFilters();

		await expect
			.element(page.getByRole('checkbox', { name: 'Show only shared vMCPs' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('checkbox', { name: 'Show only my vMCPs' })).toBeVisible();
	});

	it('clears the other ownership filter when one is checked', async () => {
		await renderSettings([Group.ADMIN]);
		await openFilters();

		const shared = page.getByRole('checkbox', { name: 'Show only shared vMCPs' });
		const mine = page.getByRole('checkbox', { name: 'Show only my vMCPs' });

		await shared.click();
		await expect.element(shared).toBeChecked();
		await expect.element(mine).not.toBeChecked();

		await mine.click();
		await expect.element(mine).toBeChecked();
		await expect.element(shared).not.toBeChecked();
	});
});
