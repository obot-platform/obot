import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import type { PageData } from './$types';
import VMcpDetailPage from './+page.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub'
});

const vmcp = createVMCP({ id: 'vmcp-1', displayName: 'Issue Tracker vMCP' }, [componentEntry]);

describe('vMCP detail page', () => {
	it('opens the designer for the loaded vMCP', async () => {
		mcpServersAndEntries.current = {
			entries: [componentEntry],
			servers: [],
			userInstances: [],
			userConfiguredServers: [],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};
		const data = await preparePageData<PageData>({ vmcp });
		render(VMcpDetailPage, { data });

		await expect
			.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'GitHub', exact: true })).toBeVisible();
	});
});
