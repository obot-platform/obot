import type { MCPCatalogServer } from '$lib/services';
import { load } from './+page';
import { describe, expect, it, vi } from 'vitest';

function server(): MCPCatalogServer {
	return {
		id: 'server-1',
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
			name: 'Route test server',
			runtime: 'npx',
			serverUserType: 'singleUser'
		}
	} as MCPCatalogServer;
}

function routeFetcher(deployments: MCPCatalogServer[]) {
	return vi.fn<typeof fetch>(async (input) => {
		const pathname = new URL(String(input)).pathname;
		if (pathname === '/api/mcp-servers') {
			return Response.json({ items: deployments });
		}
		if (pathname === '/api/all-mcps/servers') {
			return Response.json({ items: [] });
		}
		return new Response('unexpected request', { status: 500 });
	});
}

describe('MCP tester route load', () => {
	it('derives a server-owned management route for Back navigation', async () => {
		const deployment = server();
		const result = await load({
			params: { id: deployment.id },
			fetch: routeFetcher([deployment])
		} as unknown as Parameters<typeof load>[0]);

		expect(result).toMatchObject({
			server: { id: deployment.id, canConnect: true },
			backTarget: `/mcp-servers/c/${deployment.catalogEntryID}/instance/${deployment.id}`
		});
	});

	it('uses the existing 404 route convention for invalid or access-filtered IDs', async () => {
		await expect(
			load({
				params: { id: 'management-only-server' },
				fetch: routeFetcher([])
			} as unknown as Parameters<typeof load>[0])
		).rejects.toMatchObject({ status: 404 });
	});
});
