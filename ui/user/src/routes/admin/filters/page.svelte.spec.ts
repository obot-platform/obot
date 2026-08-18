import type { MCPFilter } from '$lib/services';
import { preparePageData } from '../../../tests/helpers/pageData';
import type { PageData } from './$types';
import FiltersPage from './+page.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createFilter(overrides: Partial<MCPFilter>): MCPFilter {
	return {
		configured: true,
		created: '2026-08-18T00:00:00Z',
		hasSecret: false,
		id: 'mwv1filter',
		name: 'Filter',
		type: 'mcpWebhookValidation',
		url: 'https://example.com/filter',
		...overrides
	};
}

async function renderFiltersPage() {
	const data = await preparePageData<PageData>({
		filters: [
			createFilter({
				id: 'mwv1mcp',
				name: 'MCP Filter',
				resources: [{ type: 'mcpCatalog', id: 'default' }]
			}),
			createFilter({
				id: 'mwv1device',
				localAgentEvents: ['*'],
				name: 'Device Filter',
				resources: [
					{ type: 'selector', id: '*' },
					{ type: 'deviceSelector', id: '*' }
				]
			})
		],
		systemCatalogEntries: []
	});

	return render(FiltersPage, { data });
}

describe('Filters Page', () => {
	it('summarizes MCP targets and All Devices separately', async () => {
		await renderFiltersPage();

		const deviceRow = page.getByRole('row').filter({ hasText: 'Device Filter' });
		await expect.element(deviceRow.getByText('1 target', { exact: true })).toBeVisible();
		await expect.element(deviceRow.getByText('All Devices', { exact: true })).toBeVisible();

		const mcpRow = page.getByRole('row').filter({ hasText: 'MCP Filter' });
		await expect.element(mcpRow.getByText('1 target', { exact: true })).toBeVisible();
		await expect.element(mcpRow.getByText('All Devices', { exact: true })).not.toBeInTheDocument();
	});
});
