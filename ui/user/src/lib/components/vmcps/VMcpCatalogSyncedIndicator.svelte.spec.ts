import { createVMCP } from '../../../tests/helpers/mcp';
import VMcpCatalogSyncedIndicator from './VMcpCatalogSyncedIndicator.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function indicator() {
	return page.getByRole('button', { name: 'Synced from catalog' });
}

describe('VMcpCatalogSyncedIndicator.svelte', () => {
	it('explains that a catalog-synced vMCP is managed by its source', async () => {
		const sourceURL = 'https://github.com/example/catalog';
		render(VMcpCatalogSyncedIndicator, { vmcp: { ...createVMCP(), sourceURL } });

		(await indicator().element()).focus();

		await expect
			.element(page.getByText(/managed by a catalog source and is read-only/))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: sourceURL }))
			.toHaveAttribute('href', sourceURL);
	});

	it('shows a non-web source as text', async () => {
		render(VMcpCatalogSyncedIndicator, {
			vmcp: { ...createVMCP(), sourceURL: 'git@github.com:example/catalog.git' }
		});

		(await indicator().element()).focus();

		await expect
			.element(page.getByText('Source: git@github.com:example/catalog.git'))
			.toBeVisible();
		await expect.element(page.getByRole('link')).not.toBeInTheDocument();
	});

	it('renders nothing for a vMCP created in Obot', async () => {
		render(VMcpCatalogSyncedIndicator, { vmcp: createVMCP() });

		await expect.element(indicator()).not.toBeInTheDocument();
	});
});
