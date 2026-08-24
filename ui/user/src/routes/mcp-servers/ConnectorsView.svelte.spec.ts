import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createMCPCatalogServer } from '../../tests/helpers/mcp';
import ConnectorsView from './ConnectorsView.svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const description = 'A description that remains visible when the server has status badges.';

describe('MCP Servers ConnectorsView', () => {
	beforeEach(() => {
		const entry = createMCPCatalogEntry({
			id: 'deprecated-connected-entry',
			name: 'Deprecated Connected Server',
			manifest: {
				description,
				metadata: { deprecated: 'true' }
			}
		});
		const configuredServer = createMCPCatalogServer({
			id: 'configured-server',
			name: entry.manifest.name!,
			catalogEntryID: entry.id,
			userID: 'user-1'
		});

		mcpServersAndEntries.current = {
			entries: [entry],
			servers: [],
			userInstances: [],
			userConfiguredServers: [configuredServer],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};
	});

	it('clamps the description independently from the title and status badges', async () => {
		render(ConnectorsView, { query: 'Deprecated' });

		const descriptionPreview = page.getByText(description, { exact: true });
		await expect.element(page.getByText('Deprecated', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Connected', { exact: true })).toBeVisible();
		await expect.element(descriptionPreview).toHaveClass(/line-clamp-2/);
		await expect.element(descriptionPreview.locator('..')).not.toHaveClass(/line-clamp-2/);
	});
});
