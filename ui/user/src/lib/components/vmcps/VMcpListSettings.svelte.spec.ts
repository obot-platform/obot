import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import VMcpListSettings from './VMcpListSettings.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const defaultFilters = {
	showMyVMcpsOnly: false,
	sortBy: 'name' as const,
	sortDirection: 'asc' as const,
	query: '',
	componentFilterBy: '',
	statusFilterBy: '',
	creatorFilterBy: '',
	variant: 'grid' as const
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

		await expect.element(page.getByRole('checkbox', { name: 'Show Only My vMCPs' })).toBeVisible();
	});

	it('shows the status filter options', async () => {
		await renderSettings();
		await openFilters();

		await expect.element(page.getByText(/^Filter By Status$/)).toBeVisible();
		await expect.element(page.getByRole('combobox', { name: 'Filter By Status' })).toBeVisible();
	});

	it('hides show only my vMCPs for admins and offers a creator filter', async () => {
		await renderSettings([Group.ADMIN]);
		await openFilters();

		await expect
			.element(page.getByRole('checkbox', { name: 'Show Only My vMCPs' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText(/^Filter By Creator$/)).toBeVisible();
		await expect.element(page.getByRole('combobox', { name: 'Filter By Creator' })).toBeVisible();
	});

	it('writes sortBy and direction when a sort option is selected', async () => {
		const onChange = vi.fn();
		await preparePageData({
			profile: createMockProfile([Group.USER])
		});
		render(VMcpListSettings, {
			filters: defaultFilters,
			onChange
		});
		await openFilters();
		await page.getByRole('combobox', { name: 'Sort By' }).click();
		await page.getByRole('button', { name: 'Created', exact: true }).click();
		await page.getByRole('combobox', { name: 'Sort Direction' }).click();
		await page.getByRole('button', { name: 'Descending', exact: true }).click();

		expect(onChange).toHaveBeenCalledWith('sortBy', ['created']);
		expect(onChange).toHaveBeenCalledWith('sortDirection', ['desc']);
	});

	it('shows an active filter pill when show my vMCPs only is enabled', async () => {
		await preparePageData({
			profile: createMockProfile([Group.USER])
		});
		await render(VMcpListSettings, {
			filters: { ...defaultFilters, showMyVMcpsOnly: true },
			onChange: () => {}
		});

		await expect.element(page.getByText('Show Only My vMCPs').first()).toBeVisible();
	});
});
