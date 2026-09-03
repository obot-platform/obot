import { getMCPTesterServer, listMCPs } from './operations';
import type { MCPCatalogServer } from './types';
import { describe, expect, it, vi } from 'vitest';

describe('listMCPs', () => {
	it('requests a minimal entry response when requested', async () => {
		const fetcher = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ items: [] }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			})
		);

		await listMCPs({ fetch: fetcher, minimal: true });

		expect(fetcher).toHaveBeenCalledOnce();
		expect(fetcher.mock.calls[0][0]).toMatch(/\/api\/all-mcps\/entries\?minimal=true$/);
	});

	it('keeps the full response as the default', async () => {
		const fetcher = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ items: [] }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			})
		);

		await listMCPs({ fetch: fetcher });

		expect(fetcher.mock.calls[0][0]).toMatch(/\/api\/all-mcps\/entries$/);
	});
});

function server(id: string, overrides: Partial<MCPCatalogServer> = {}): MCPCatalogServer {
	return {
		id,
		userID: 'user-1',
		configured: true,
		catalogEntryID: 'entry-1',
		missingRequiredEnvVars: [],
		mcpCatalogID: 'default',
		created: '2026-01-01T00:00:00Z',
		updated: '2026-01-01T00:00:00Z',
		type: 'mcpserver',
		serverUserType: 'singleUser',
		manifest: {
			name: 'Test server',
			runtime: 'npx',
			serverUserType: 'singleUser'
		},
		...overrides
	} as MCPCatalogServer;
}

function listFetcher(personal: MCPCatalogServer[], shared: MCPCatalogServer[]) {
	return vi.fn<typeof fetch>(async (input) => {
		const pathname = new URL(String(input)).pathname;
		if (pathname === '/api/mcp-servers') {
			return Response.json({ items: personal });
		}
		if (pathname === '/api/all-mcps/servers') {
			return Response.json({ items: shared });
		}
		return new Response('unexpected request', { status: 500 });
	});
}

describe('getMCPTesterServer', () => {
	it('returns an explicitly connectable deployment from user-scoped listings', async () => {
		const deployment = server('shared-server', {
			manifest: {
				name: 'Composite gateway',
				runtime: 'composite'
			}
		});
		const fetcher = listFetcher([], [deployment]);

		await expect(getMCPTesterServer(deployment.id, { fetch: fetcher })).resolves.toMatchObject({
			id: deployment.id,
			canConnect: true
		});
		expect(fetcher).toHaveBeenCalledTimes(2);
	});

	it('rejects absent, deleted, and internal composite-component IDs', async () => {
		const fetcher = listFetcher(
			[server('deleted-server', { deleted: '2026-01-02T00:00:00Z' })],
			[server('component-server', { compositeName: 'composite-parent' })]
		);

		await expect(
			getMCPTesterServer('management-only-server', { fetch: fetcher })
		).rejects.toMatchObject({ statusCode: 404 });
		await expect(getMCPTesterServer('deleted-server', { fetch: fetcher })).rejects.toMatchObject({
			statusCode: 404
		});
		await expect(getMCPTesterServer('component-server', { fetch: fetcher })).rejects.toMatchObject({
			statusCode: 404
		});
	});
});
