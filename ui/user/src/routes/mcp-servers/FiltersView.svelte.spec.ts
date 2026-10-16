import type { MCPFilter } from '$lib/services';
import FiltersView from './FiltersView.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createFilter(overrides: Partial<MCPFilter>): MCPFilter {
	return {
		id: 'mwv1filter',
		created: '2026-01-01T00:00:00Z',
		type: 'mcpwebhookvalidation',
		hasSecret: false,
		configured: true,
		resources: [{ id: 'default', type: 'mcpCatalog' }],
		selectors: [],
		...overrides
	};
}

const filters: MCPFilter[] = [
	createFilter({
		id: 'mwv1guardrail',
		name: 'Guardrail filter',
		guardrail: { guardrailIDs: ['gp1secrets'], action: 'block' }
	}),
	createFilter({ id: 'mwv1webhook', name: 'Webhook filter', url: 'https://example.com/hook' }),
	createFilter({
		id: 'mwv1builtin',
		name: 'Built-in filter',
		systemMCPServerCatalogEntryID: 'sce1builtin',
		mcpServerManifest: { runtime: 'containerized' }
	}),
	createFilter({
		id: 'mwv1server',
		name: 'Server filter',
		toolName: 'check',
		mcpServerManifest: { runtime: 'npx', npxConfig: { package: 'filter-pkg', args: [] } }
	})
];

describe('FiltersView.svelte', () => {
	it('shows each filter type in the Type column', async () => {
		await render(FiltersView, { filters });

		await expect.element(page.getByRole('button', { name: 'Type', exact: true })).toBeVisible();
		for (const [name, type] of [
			['Guardrail filter', 'Guardrail'],
			['Webhook filter', 'Webhook'],
			['Built-in filter', 'Built-in'],
			['Server filter', 'MCP server']
		]) {
			const row = page.getByRole('row').filter({ hasText: name });
			await expect.element(row.getByRole('cell', { name: type, exact: true })).toBeVisible();
		}
	});
});
