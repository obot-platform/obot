import { createMcpServerDetailsFixtures } from '../../../tests/mocks/data';
import { page } from 'vitest/browser';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import McpOauth from './McpOauth.svelte';
import { HttpResponse, http } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

it.each([
	{
		name: 'standalone',
		scope: { mcpCatalogID: '' },
		path: '/api/mcp-servers/ms1test/oauth-url'
	},
	{
		name: 'catalog',
		scope: { mcpCatalogID: 'default' },
		path: '/api/mcp-catalogs/default/servers/ms1test/oauth-url'
	},
	{
		name: 'workspace',
		scope: { mcpCatalogID: '', powerUserWorkspaceID: 'workspace1' },
		path: '/api/workspaces/workspace1/servers/ms1test/oauth-url'
	}
])(
	'requests the OAuth URL for a $name server from its scoped endpoint',
	async ({ scope, path }) => {
		const requestedPaths: string[] = [];
		worker.use(
			http.get('/api/*/oauth-url', ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({ oauthURL: '' });
			})
		);
		const server = createMCPCatalogServer({
			id: 'ms1test',
			name: 'Test Server',
			runtime: 'remote',
			serverUserType: 'multiUser',
			userID: 'user1',
			...scope
		});

		render(McpOauth, { entry: server });
		await expect.poll(() => requestedPaths).toEqual([path]);
	}
);

it('uses the pending UI attempt and checks authentication before resuming', async () => {
	const entry = structuredClone(createMcpServerDetailsFixtures().serverSingle);
	entry.vmcpID = 'vmcp1parent';
	entry.connectURL = 'https://obot.example/mcp-connect/deployed-server';
	entry.manifest.remoteConfig = { url: 'https://mcp.example.com', localhostCallbackEnabled: true };
	const oauthRequest = vi.fn();
	const attemptURL = 'https://obot.example/oauth/mcp/login/preview-state';
	worker.use(
		http.get(`/api/oauth/vmcp/vmcp1parent/components/${entry.id}`, () => {
			oauthRequest();
			return HttpResponse.json({
				authURL: oauthRequest.mock.calls.length === 1 ? attemptURL : ''
			});
		})
	);
	const onAuthenticate = vi.fn();
	await render(McpOauth, { entry, onAuthenticate });
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${attemptURL}'`);
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
	await vi.waitFor(() => expect(onAuthenticate).toHaveBeenCalledOnce());
	expect(oauthRequest).toHaveBeenCalledTimes(2);
});

it('shows endpoint failures and retries the same component without treating errors as login success', async () => {
	const entry = structuredClone(createMcpServerDetailsFixtures().serverSingle);
	entry.vmcpInstanceID = 'vmcpi1exact';
	let calls = 0;
	worker.use(
		http.get(`/api/oauth/vmcp/vmcpi1exact/components/${entry.id}`, () => {
			if (++calls === 1)
				return HttpResponse.json({ message: 'Provider unavailable' }, { status: 503 });
			return HttpResponse.json({ authURL: 'https://obot.example/oauth/mcp/login/retry-attempt' });
		})
	);
	const onAuthenticate = vi.fn();
	await render(McpOauth, { entry, onAuthenticate });
	await expect
		.element(page.getByRole('alert').getByText('Provider unavailable', { exact: false }))
		.toBeVisible();
	expect(onAuthenticate).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Retry authentication' }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent("obot mcp login --url 'https://obot.example/oauth/mcp/login/retry-attempt'");
	expect(onAuthenticate).not.toHaveBeenCalled();
	expect(calls).toBe(2);
});
