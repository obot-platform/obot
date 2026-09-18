import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import VMcpListSettings from './VMcpListSettings.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const defaultFilters = {
	showMyVMcpsOnly: false,
	sortBy: 'name' as const,
	query: '',
	componentFilterBy: '',
	statusFilterBy: ''
};

async function renderSettings(groups: string[] = [Group.ADMIN]) {
	await preparePageData({
		profile: createMockProfile(groups)
	});
	return render(VMcpListSettings, {
		filters: defaultFilters,
		onChange: () => {}
	});
}

async function openFilters() {
	await page.getByRole('button', { name: 'Filters' }).click();
	await expect.element(page.getByRole('heading', { name: 'vMCPs Settings' })).toBeVisible();
}

describe('VMcpListSettings.svelte', () => {
	it('shows the my vMCPs creator filter', async () => {
		await renderSettings([Group.USER]);
		await openFilters();

		await expect.element(page.getByRole('checkbox', { name: 'Show my vMCPs only' })).toBeVisible();
	});

	it('shows the status filter options', async () => {
		await renderSettings();
		await openFilters();

		await expect.element(page.getByText(/^Filter By Status$/)).toBeVisible();
		await expect.element(page.getByRole('combobox', { name: 'Filter By Status' })).toBeVisible();
	});
});
