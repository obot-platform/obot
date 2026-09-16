import { goto } from '$app/navigation';
import { errors, mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import DeploymentsView from './DeploymentsView.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => ({
	...(await importOriginal<typeof import('$app/navigation')>()),
	goto: vi.fn()
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
		skipLoadOnMount: true,
		onReload: () => {}
	});

	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: 'View vMCP Deployment', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: 'View vMCP Deployment', exact: true }))
		.not.toBeInTheDocument();
	await vi.waitFor(() =>
		expect(goto).toHaveBeenCalledWith('/vmcps/vmcp1-test/instance/vmcpi1-test')
	);
	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: 'View vMCP', exact: true }).click();
	await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/vmcps/vmcp1-test'));
	expect(vi.mocked(goto).mock.calls).toEqual([
		['/vmcps/vmcp1-test/instance/vmcpi1-test'],
		['/vmcps/vmcp1-test']
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
		skipLoadOnMount: true
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
	errors.items = [];
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
		skipLoadOnMount: true
	});
	await page.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: action, exact: true }).click();
	await vi.waitFor(() => {
		expect(errors.items.map((error) => error.message)).toEqual([
			action === 'View vMCP' ? 'Failed to open vMCP.' : 'Failed to open vMCP deployment.'
		]);
	});
	expect(goto).not.toHaveBeenCalled();
	errors.items = [];
});
