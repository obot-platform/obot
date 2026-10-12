import {
	exchangeMCPServerOAuthDebuggerToken,
	getMCPServerOAuthDebuggerAuthorizationURL,
	registerMcpServerOAuthDebuggerClient
} from './operations';
import { afterEach, expect, it, vi } from 'vitest';

afterEach(() => vi.unstubAllGlobals());

it.each([
	{
		name: 'standalone',
		server: { id: 'ms1test', mcpCatalogID: '' },
		base: '/api/mcp-servers/ms1test'
	},
	{
		name: 'catalog',
		server: { id: 'ms1test', mcpCatalogID: 'default' },
		base: '/api/mcp-catalogs/default/servers/ms1test'
	},
	{
		name: 'workspace',
		server: { id: 'ms1test', mcpCatalogID: '', powerUserWorkspaceID: 'workspace1' },
		base: '/api/workspaces/workspace1/servers/ms1test'
	}
])(
	'routes OAuth debugger actions for a $name server to its scoped endpoints',
	async ({ server, base }) => {
		const fetcher = vi.fn().mockImplementation(() =>
			Promise.resolve(
				new Response(JSON.stringify({}), {
					status: 200,
					headers: { 'content-type': 'application/json' }
				})
			)
		);
		vi.stubGlobal('fetch', fetcher);
		await registerMcpServerOAuthDebuggerClient(server);
		await getMCPServerOAuthDebuggerAuthorizationURL(server, { state: 'state' });
		await exchangeMCPServerOAuthDebuggerToken(server, { code: 'code', state: 'state' });
		expect(fetcher.mock.calls.map(([url]) => new URL(url, 'http://localhost').pathname)).toEqual([
			`${base}/oauth-debugger/client`,
			`${base}/oauth-debugger/authorization-url`,
			`${base}/oauth-debugger/token`
		]);
	}
);
