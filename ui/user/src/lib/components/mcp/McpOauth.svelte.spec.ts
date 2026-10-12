import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import McpOauth from './McpOauth.svelte';
import { HttpResponse, http } from 'msw';
import { expect, it } from 'vitest';
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
