import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import SearchMcpServers from './SearchMcpServers.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('omits vMCP component servers from access policy server choices', async () => {
	const entry = createMCPCatalogEntry({ id: 'entry1outlook', name: 'Outlook' });
	const multiUserServer = createMCPCatalogServer({
		id: 'ms1shared',
		name: 'Shared Outlook',
		serverUserType: 'multiUser',
		userID: 'user1'
	});
	const sharedComponent = createMCPCatalogServer({
		id: 'ms1vmcpshared',
		name: 'Outlook',
		catalogEntryID: entry.id,
		vmcpID: 'vmcp1mail',
		vmcpComponentID: 'outlook',
		userID: 'user1'
	});
	const instanceComponent = createMCPCatalogServer({
		id: 'ms1vmcpinstance',
		name: 'Outlook',
		catalogEntryID: entry.id,
		vmcpInstanceID: 'vmcpi1mail',
		vmcpComponentID: 'outlook',
		userID: 'user1'
	});
	const onAdd = vi.fn();

	const { component } = await render(SearchMcpServers, {
		type: 'acr',
		onAdd,
		mcpEntriesContextFn: () => ({
			entries: [entry],
			servers: [multiUserServer, sharedComponent, instanceComponent],
			loading: false
		})
	});
	component.open();
	await page.getByPlaceholder('Search by name...').fill('Outlook');

	await expect.element(page.getByRole('button', { name: /Shared Outlook/ })).toBeVisible();
	await expect.element(page.getByRole('button', { name: /^Outlook/ })).toBeVisible();
	expect(page.getByRole('button', { name: /^Outlook/ }).elements()).toHaveLength(1);
	expect(document.getElementById(`search-mcp-server-${sharedComponent.id}`)).toBeNull();
	expect(document.getElementById(`search-mcp-server-${instanceComponent.id}`)).toBeNull();
});
