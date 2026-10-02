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

it('uses CLI login and retries listing tools without requesting a browser OAuth URL', async () => {
	const entry = structuredClone(createMcpServerDetailsFixtures().serverSingle);
	entry.connectURL = 'https://obot.example/mcp-connect/deployed-server';
	entry.manifest.remoteConfig = { url: 'https://mcp.example.com', localhostCallbackEnabled: true };
	const oauthRequest = vi.fn();
	worker.use(
		http.get('*/api/*/oauth-url', () => {
			oauthRequest();
			return HttpResponse.json({ oauthURL: 'https://auth.example/authorize' });
		})
	);
	const onAuthenticate = vi.fn();
	await render(McpOauth, { entry, onAuthenticate });
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${entry.connectURL}'`);
	await page.getByRole('button', { name: 'Retry', exact: true }).click();
	expect(onAuthenticate).toHaveBeenCalledOnce();
	expect(oauthRequest).not.toHaveBeenCalled();
});
