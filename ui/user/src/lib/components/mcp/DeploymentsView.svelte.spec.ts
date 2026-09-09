import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
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
