import {
	createMCPCatalogEntry,
	createMCPCatalogServer,
	createVMCP
} from '../../../tests/helpers/mcp';
import SearchMcpServers from './SearchMcpServers.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// Shared deployments only exist as vMCP components, which are reachable only through their vMCP.
function outlookFixtures() {
	const entry = createMCPCatalogEntry({ id: 'entry1outlook', name: 'Outlook' });
	const vmcp = createVMCP({ id: 'vmcp1mail', displayName: 'Mail Gateway' });
	const servers = [
		createMCPCatalogServer({
			id: 'ms1vmcpshared',
			name: 'Outlook',
			catalogEntryID: entry.id,
			serverUserType: 'multiUser',
			vmcpID: vmcp.id,
			vmcpComponentID: 'outlook',
			userID: 'user1'
		}),
		createMCPCatalogServer({
			id: 'ms1vmcpinstance',
			name: 'Outlook',
			catalogEntryID: entry.id,
			vmcpInstanceID: 'vmcpi1mail',
			vmcpComponentID: 'outlook',
			userID: 'user1'
		})
	];
	const context = { entries: [entry], servers, loading: false };
	return { entry, vmcp, context };
}

async function searchFor(name: string) {
	await page.getByPlaceholder('Search by name...').fill(name);
}

it('lists only catalog entries as access policy server choices', async () => {
	const { entry, context } = outlookFixtures();
	const onAdd = vi.fn();
	const { component } = await render(SearchMcpServers, {
		type: 'acr',
		onAdd,
		mcpEntriesContextFn: () => context
	});
	component.open();
	await searchFor('Outlook');

	await expect.element(page.getByRole('button', { name: /^Outlook/ })).toBeVisible();
	expect(page.getByRole('button', { name: /^Outlook/ }).elements()).toHaveLength(1);

	await page.getByRole('button', { name: /^Outlook/ }).click();
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	expect(onAdd).toHaveBeenCalledWith([entry.id], [], []);
});

it('lists vMCPs but not their component servers as filter choices', async () => {
	const { vmcp, context } = outlookFixtures();
	const { component } = await render(SearchMcpServers, {
		type: 'filter',
		onAdd: vi.fn(),
		vmcps: [vmcp],
		mcpEntriesContextFn: () => context
	});
	component.open();

	await searchFor('Mail Gateway');
	await expect.element(page.getByRole('button', { name: /Mail Gateway/ })).toBeVisible();

	await searchFor('Outlook');
	await expect.element(page.getByRole('button', { name: /^Outlook/ })).toBeVisible();
	expect(page.getByRole('button', { name: /^Outlook/ }).elements()).toHaveLength(1);
});
