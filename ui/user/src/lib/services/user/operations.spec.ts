import {
	configureVMCPInstance,
	createVMCP,
	createVMCPInstance,
	deleteVMCP,
	deleteVMCPInstance,
	getMCPTesterServer,
	getVMCP,
	getVMCPInstance,
	listMCPs,
	listVMCPInstances,
	listVMCPs,
	updateVMCP
} from './operations';
import type { MCPCatalogServer, VMCP, VMCPConfiguration, VMCPManifest } from './types';
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

const manifest: VMCPManifest = {
	displayName: 'Issue Gateway',
	description: 'Cached catalog snapshots',
	icon: '',
	components: [
		{
			id: 'component-github',
			name: 'GitHub',
			mcpCatalogID: 'default',
			mcpServerCatalogEntryID: 'entry-github',
			catalogEntry: {
				manifest: {
					name: 'GitHub',
					runtime: 'npx',
					npxConfig: { package: '@modelcontextprotocol/server-github' },
					toolPreview: [{ id: 'create_issue', name: 'create_issue' }]
				},
				unsupportedTools: []
			}
		}
	],
	profiles: [
		{
			name: 'default',
			subjects: [{ type: 'selector', id: '*' }],
			allowAllTools: true
		}
	],
	forceSingleUser: false
};

const vmcp: VMCP = {
	...manifest,
	id: 'vmcp-1',
	created: '2026-01-01T00:00:00.000Z',
	type: 'vmcp',
	status: { ready: true }
};

function response(body: unknown) {
	return Promise.resolve(
		new Response(JSON.stringify(body), {
			status: 200,
			headers: { 'content-type': 'application/json' }
		})
	);
}

describe('vMCP operations', () => {
	it('uses the first-class vMCP collection and resource endpoints', async () => {
		const fetcher = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url.endsWith('/api/vmcps') && (!init?.method || init.method === 'GET')) {
				return response({ items: [vmcp] });
			}
			if (url.endsWith('/api/vmcps/vmcp-1') && (!init?.method || init.method === 'GET')) {
				return response(vmcp);
			}
			if (url.endsWith('/api/vmcps/vmcp-1') && init?.method === 'PUT') {
				return response(vmcp);
			}
			return response({});
		});

		expect(await listVMCPs({ fetch: fetcher })).toEqual([vmcp]);
		expect(await getVMCP(vmcp.id, { fetch: fetcher })).toEqual(vmcp);
		expect(await updateVMCP(vmcp.id, manifest, { fetch: fetcher })).toEqual(vmcp);

		expect(
			fetcher.mock.calls.map(([input]) => String(input).replace('http://localhost:8080', ''))
		).toEqual(['/api/vmcps', '/api/vmcps/vmcp-1', '/api/vmcps/vmcp-1']);
		expect(fetcher.mock.calls[2][1]).toMatchObject({
			method: 'PUT',
			body: JSON.stringify(manifest)
		});
	});

	it('posts and deletes vMCP resources without using catalog-entry routes', async () => {
		const fetcher = vi.fn().mockImplementation(() => response(vmcp));

		expect(await createVMCP(manifest, { fetch: fetcher })).toEqual(vmcp);
		await deleteVMCP(vmcp.id, { fetch: fetcher });

		expect(String(fetcher.mock.calls[0][0])).toMatch(/\/api\/vmcps$/);
		expect(fetcher.mock.calls[0][1]).toMatchObject({
			method: 'POST',
			body: JSON.stringify(manifest)
		});
		expect(String(fetcher.mock.calls[1][0])).toMatch(/\/api\/vmcps\/vmcp-1$/);
		expect(fetcher.mock.calls[1][1]).toMatchObject({ method: 'DELETE' });
		for (const [input] of fetcher.mock.calls) {
			expect(String(input)).not.toContain('/mcp-catalogs/');
		}
	});

	it('uses the vMCP instance endpoints for idempotent connect and configure flows', async () => {
		const instance = {
			id: 'instance-1',
			created: '2026-01-01T00:00:00.000Z',
			type: 'vmcpinstance',
			vmcpID: vmcp.id,
			userID: 'user-1',
			status: { configured: false }
		};
		const configuration: VMCPConfiguration = {
			components: { 'component-github': { TOKEN: 'redacted' } }
		};
		const fetcher = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url.endsWith('/api/vmcp-instances') && init?.method === 'POST') {
				return response(instance);
			}
			if (url.endsWith('/api/vmcp-instances/instance-1/configure')) return response(instance);
			if (
				url.endsWith('/api/vmcp-instances/instance-1') &&
				(!init?.method || init.method === 'GET')
			) {
				return response(instance);
			}
			if (url.endsWith('/api/vmcp-instances') && (!init?.method || init.method === 'GET')) {
				return response({ items: [instance] });
			}
			return response({});
		});

		const originalFetch = globalThis.fetch;
		vi.stubGlobal('fetch', fetcher);
		try {
			expect(await createVMCPInstance(vmcp.id)).toBeDefined();
			expect(await listVMCPInstances({ fetch: fetcher })).toEqual([instance]);
			expect(await getVMCPInstance(instance.id, { fetch: fetcher })).toEqual(instance);
			expect(await configureVMCPInstance(instance.id, configuration)).toEqual(instance);
			await deleteVMCPInstance(instance.id);
		} finally {
			vi.stubGlobal('fetch', originalFetch);
		}

		expect(
			fetcher.mock.calls.map(([input]) => String(input).replace('http://localhost:8080', ''))
		).toEqual([
			'/api/vmcp-instances',
			'/api/vmcp-instances',
			'/api/vmcp-instances/instance-1',
			'/api/vmcp-instances/instance-1/configure',
			'/api/vmcp-instances/instance-1'
		]);
		expect(fetcher.mock.calls[0][1]).toMatchObject({
			method: 'POST',
			body: JSON.stringify({ vmcpID: vmcp.id })
		});
		expect(fetcher.mock.calls[3][1]).toMatchObject({
			method: 'POST',
			body: JSON.stringify(configuration)
		});
		expect(fetcher.mock.calls[4][1]).toMatchObject({ method: 'DELETE' });
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

	it('rejects absent, deleted, composite-component, and template IDs', async () => {
		const fetcher = listFetcher(
			[server('deleted-server', { deleted: '2026-01-02T00:00:00Z' })],
			[
				server('component-server', { compositeName: 'composite-parent' }),
				server('template-server', { template: true })
			]
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
		await expect(getMCPTesterServer('template-server', { fetch: fetcher })).rejects.toMatchObject({
			statusCode: 404
		});
	});
});
