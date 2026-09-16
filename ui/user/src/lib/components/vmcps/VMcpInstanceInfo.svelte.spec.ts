import { mcpServersAndEntries } from '$lib/stores';
import { openUrl } from '$lib/utils';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import VMcpInstanceInfo from './VMcpInstanceInfo.svelte';
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
