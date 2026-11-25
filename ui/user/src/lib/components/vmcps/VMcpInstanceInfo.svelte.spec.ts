import { mcpServersAndEntries } from '$lib/stores';
import { openUrl } from '$lib/utils';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import VMcpInstanceInfo from './VMcpInstanceInfo.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/utils', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/utils')>()),
	openUrl: vi.fn()
}));

it.each([
	{
		policy: 'userAllowed' as const,
		path: '/mcp-servers/c/github/instance/ms1vmcpi1-test-component-github/details'
	},
	{ policy: 'fixed' as const, path: '/mcp-servers/s/ms1vmcp1-test-component-github/details' },
	{
		policy: 'userAllowed' as const,
		workspaceID: 'workspace/test',
		path: '/mcp-servers/c/github/instance/ms1vmcpi1-test-component-github/details?wid=workspace%2Ftest'
	}
])(
	'opens the correct component deployment for $policy configuration',
	async ({ policy, path, workspaceID }) => {
		const entry = createMCPCatalogEntry({
			id: 'github',
			name: 'GitHub',
			powerUserWorkspaceID: workspaceID
		});
		const vmcp = createVMCP({ id: 'vmcp1-test' }, [entry]);
		vmcp.components[0].configuration = [{ key: 'API_TOKEN', policy }];
		mcpServersAndEntries.current = {
			entries: [entry],
			servers: [],
			userInstances: [],
			userConfiguredServers: [],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};
		vi.mocked(openUrl).mockClear();

		render(VMcpInstanceInfo, {
			vmcp,
			instance: {
				id: 'vmcpi1-test',
				vmcpID: vmcp.id,
				userID: 'user-1',
				created: '2026-01-01T00:00:00Z',
				status: {}
			}
		});

		await page.getByRole('button', { name: /GitHub/ }).click();
		expect(openUrl).toHaveBeenCalledWith(path, false);
	}
);

it.each([
	{ owner: 'agent:7', name: 'Build Bot' },
	{ owner: 'agent:9', name: 'Old Bot (Deleted)' }
])('names the agent that owns the instance: $name', async ({ owner, name }) => {
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
	const entry = createMCPCatalogEntry({ id: 'github', name: 'GitHub' });
	const vmcp = createVMCP({ id: 'vmcp1-test' }, [entry]);

	render(VMcpInstanceInfo, {
		vmcp,
		instance: {
			id: 'vmcpi1-test',
			vmcpID: vmcp.id,
			userID: owner,
			created: '2026-01-01T00:00:00Z',
			status: {}
		}
	});

	await expect.element(page.getByText(name, { exact: true })).toBeVisible();
	await expect.element(page.getByText('Agent', { exact: true })).toBeVisible();
	await expect.element(page.getByText(owner)).not.toBeInTheDocument();
});
