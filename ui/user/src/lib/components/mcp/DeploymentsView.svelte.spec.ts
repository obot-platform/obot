import { goto } from '$app/navigation';
import type { OrgUser } from '$lib/services';
import { notifications, mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import DeploymentsView from './DeploymentsView.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	invalidate: vi.fn(),
	invalidateAll: vi.fn(),
	preloadCode: vi.fn(),
	preloadData: vi.fn(),
	beforeNavigate: vi.fn(),
	afterNavigate: vi.fn(),
	onNavigate: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	disableScrollHandling: vi.fn()
}));

it('links dedicated components by instance ID without loading deployment data', async () => {
	vi.mocked(goto).mockClear();
	worker.use(
		http.get('/api/vmcp-instances/vmcpi1-test', () =>
			HttpResponse.json({ id: 'vmcpi1-test', vmcpID: 'vmcp1-test' })
		)
	);
	const server = createMCPCatalogServer({
		id: 'component-deployment',
		name: 'Dedicated component',
		userID: 'user-1',
		vmcpInstanceID: 'vmcpi1-test',
		vmcpComponentID: 'component-1'
	});

	render(DeploymentsView, {
		servers: [server],
		skipLoadMcpsOnMount: true,
		onReload: () => {}
	});

	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: 'View vMCP Deployment', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'View vMCP Deployment', exact: true }))
		.not.toBeInTheDocument();
	await vi.waitFor(() =>
		expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/vmcps/vmcp1-test/instance/vmcpi1-test')
	);
	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: 'View vMCP', exact: true }).click();
	await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[1]?.[0]).toBe('/vmcps/vmcp1-test'));
	expect(vi.mocked(goto).mock.calls.map(([url]) => url)).toEqual([
		'/vmcps/vmcp1-test/instance/vmcpi1-test',
		'/vmcps/vmcp1-test'
	]);
});

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
		skipLoadMcpsOnMount: true
	});

	await expect.element(page.getByText('Active deployment', { exact: true }).first()).toBeVisible();
	await expect.element(page.getByText('Legacy component', { exact: true })).not.toBeInTheDocument();
});

it.each(['View vMCP', 'View vMCP Deployment'])('shows one error when %s fails', async (action) => {
	worker.use(
		http.get('/api/vmcp-instances/vmcpi1-test', () =>
			HttpResponse.json({ error: 'Unavailable' }, { status: 500 })
		)
	);
	notifications.items = [];
	vi.mocked(goto).mockClear();
	render(DeploymentsView, {
		servers: [
			createMCPCatalogServer({
				id: 'component-deployment',
				name: 'Dedicated component',
				userID: 'user-1',
				vmcpInstanceID: 'vmcpi1-test',
				vmcpComponentID: 'component-1'
			})
		],
		skipLoadMcpsOnMount: true
	});
	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: action, exact: true }).click();
	await vi.waitFor(() => {
		expect(notifications.items.map((error) => error.message)).toEqual([
			action === 'View vMCP' ? 'Failed to open vMCP.' : 'Failed to open vMCP deployment.'
		]);
	});
	expect(goto).not.toHaveBeenCalled();
	notifications.items = [];
});

it('names the user, agent and deleted agent that own servers', async () => {
	worker.use(
		http.get('/api/agents', () =>
			HttpResponse.json({
				items: [
					{ id: '7', displayName: 'Build Bot', status: 'active', activeAPIKeyCount: 1 },
					{ id: '9', displayName: 'Old Bot', status: 'deleted', activeAPIKeyCount: 0 }
				]
			})
		)
	);
	const owned = (id: string, userID: string) =>
		createMCPCatalogServer({ id, name: `Server ${id}`, userID });

	render(DeploymentsView, {
		servers: [owned('s-user', 'user-1'), owned('s-agent', 'agent:7'), owned('s-gone', 'agent:9')],
		usersMap: new Map([
			['user-1', { id: 'user-1', displayName: 'Ada Admin', email: 'ada@example.com' } as OrgUser]
		]),
		skipLoadMcpsOnMount: true,
		onReload: () => {}
	});

	await expect.element(page.getByText('Ada Admin', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Build Bot', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Old Bot (Deleted)', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Agent', { exact: true }).nth(1)).toBeVisible();
	await expect.element(page.getByText('Agent', { exact: true }).nth(2)).not.toBeInTheDocument();
	await expect.element(page.getByText(/agent:/)).not.toBeInTheDocument();
});
