import type { SystemMCPServerCatalogEntry } from '$lib/services';
import BuiltInFilters from './BuiltInFilters.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const entries: SystemMCPServerCatalogEntry[] = [
	{
		id: 'pii',
		created: '2026-09-25T00:00:00Z',
		manifest: {
			name: 'PII Filter',
			shortDescription: 'Detect and redact sensitive PII',
			description: 'PII detection using Microsoft Presidio.',
			icon: '',
			runtime: 'containerized'
		}
	},
	{
		id: 'credentials',
		created: '2026-09-25T00:00:00Z',
		manifest: {
			name: 'Credential Filter',
			shortDescription: '',
			description: 'Allow, block, or redact credentials in MCP messages.',
			icon: '',
			runtime: 'containerized'
		}
	}
];

describe('built-in filter descriptions', () => {
	it('shows descriptions for multiple filters and selects the clicked entry', async () => {
		const onSelect = vi.fn();
		render(BuiltInFilters, { entries, query: '', onSelect });
		await expect
			.element(page.getByText('Detect and redact sensitive PII', { exact: true }))
			.toBeVisible();
		await expect
			.element(
				page.getByText('Allow, block, or redact credentials in MCP messages.', { exact: true })
			)
			.toBeVisible();
		await page
			.getByRole('cell', { name: 'PII Filter Detect and redact sensitive PII', exact: true })
			.click();
		expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'pii' }));
	});
	it('retains the description when searching narrows the list to one entry', async () => {
		render(BuiltInFilters, { entries, query: 'PII' });
		await expect
			.element(page.getByText('PII detection using Microsoft Presidio.', { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByText('Credential Filter', { exact: true }))
			.not.toBeInTheDocument();
	});
});
