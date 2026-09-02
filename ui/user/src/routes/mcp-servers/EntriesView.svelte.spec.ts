import { mcpServersAndEntries } from '$lib/stores';
import { openUrl } from '$lib/utils';
import { createMCPCatalogEntry } from '../../tests/helpers/mcp';
import { preparePageData } from '../../tests/helpers/pageData';
import EntriesView from './EntriesView.svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/utils', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/utils')>()),
	openUrl: vi.fn()
}));

const catalogEntry = createMCPCatalogEntry({ id: 'catalog-entry', name: 'Catalog Entry' });
const workspaceEntry = createMCPCatalogEntry({
	id: 'workspace-entry',
	name: 'Workspace Entry',
	powerUserWorkspaceID: 'ws-1'
});

async function renderEntriesView() {
	await preparePageData();
	mcpServersAndEntries.current = {
		entries: [catalogEntry, workspaceEntry],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	return render(EntriesView, { entity: 'catalog', id: 'default' });
}

async function clickRow(name: string) {
	await page.getByRole('row').filter({ hasText: name }).getByRole('cell').first().click();
}

describe('MCP Servers EntriesView', () => {
	beforeEach(() => {
		vi.mocked(openUrl).mockClear();
	});

	it('opens catalog entries without a workspace id', async () => {
		await renderEntriesView();

		await clickRow('Catalog Entry');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/c/catalog-entry', false);
	});

	it('opens workspace-owned entries scoped to their workspace', async () => {
		await renderEntriesView();

		await clickRow('Workspace Entry');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/c/workspace-entry?wid=ws-1', false);
	});

	it('keeps the workspace scope on the audit logs link', async () => {
		await renderEntriesView();

		const row = page.getByRole('row').filter({ hasText: 'Workspace Entry' });
		await row.getByRole('button', { name: 'Row actions' }).click();
		await page.getByRole('button', { name: 'View Audit Logs' }).click();

		expect(openUrl).toHaveBeenCalledWith(
			'/mcp-servers/c/workspace-entry?view=audit-logs&wid=ws-1',
			false
		);
	});
});
