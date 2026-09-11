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
	it('shows the my vMCPs ownership filter', async () => {
		await renderSettings([Group.USER]);
		await openFilters();

		await expect.element(page.getByRole('checkbox', { name: 'Show my vMCPs only' })).toBeVisible();
	});
});
