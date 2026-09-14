import { mcpServersAndEntries, profile } from '$lib/stores';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { createMockProfile } from '../../../tests/helpers/pageData';
import DeploymentsView from './DeploymentsView.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('shows deployments while hiding legacy composite children awaiting cleanup', async () => {
	mcpServersAndEntries.current = {
		entries: [],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	const server = createMCPCatalogServer({
		id: 'deployment',
		name: 'Active deployment',
		userID: 'user-1'
	});
	const legacyChild = createMCPCatalogServer({
		id: 'legacy-child',
		name: 'Legacy component',
		userID: 'user-1',
		compositeName: 'migrated-composite'
	});

	render(DeploymentsView, {
		servers: [server, legacyChild],
		entity: 'workspace',
		readonly: true,
		skipLoadOnMount: true
	});

	await expect.element(page.getByText('Active deployment', { exact: true }).first()).toBeVisible();
	await expect.element(page.getByText('Legacy component', { exact: true })).not.toBeInTheDocument();
});

it('hides the delete action for deployments owned by a vMCP', async () => {
	profile.initialize(createMockProfile());
	mcpServersAndEntries.current = {
		entries: [],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	// Explicit creation timestamps keep the default created-descending sort deterministic.
	const standalone = createMCPCatalogServer({
		id: 'standalone',
		name: 'Standalone deployment',
		userID: 'user-1',
		created: '2026-01-02T00:00:00.000Z'
	});
	const vmcpComponent = createMCPCatalogServer({
		id: 'vmcp-component',
		name: 'vMCP component',
		userID: 'user-1',
		created: '2026-01-01T00:00:00.000Z',
		vmcpName: 'vmcp1b7zz6'
	});

	render(DeploymentsView, {
		servers: [standalone, vmcpComponent],
		entity: 'workspace',
		skipLoadOnMount: true
	});

	// Both deployments stay listed; only the vMCP component loses its delete action.
	await expect.element(page.getByText('vMCP component', { exact: true }).first()).toBeVisible();

	// The open popover renders a full-screen click catcher that intercepts Playwright
	// actionability checks, so toggle the row menus with native clicks.
	const rowActions = page.getByRole('button', { name: 'Row actions' });
	const openRowMenu = async (index: number) => {
		const button = await rowActions.nth(index).element();
		(button as HTMLElement).click();
	};

	await openRowMenu(0);
	await expect.element(page.getByRole('button', { name: 'Delete Server' })).toBeVisible();
	await openRowMenu(0);

	await openRowMenu(1);
	await expect.element(page.getByRole('button', { name: 'Restart Server' })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'Delete Server' })).not.toBeInTheDocument();
});
