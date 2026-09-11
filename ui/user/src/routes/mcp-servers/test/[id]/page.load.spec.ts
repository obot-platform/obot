import type { MCPCatalogServer, VMCP, VMCPInstance } from '$lib/services';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { load } from './+page';
import { describe, expect, it, vi } from 'vitest';

const PROFILE_ID = 'user-1';

function loadTester(id: string, fetcher: typeof fetch, profileId = PROFILE_ID) {
	const profile = { ...createMockProfile(), id: profileId };
	return load({
		params: { id },
		fetch: fetcher,
		parent: vi.fn(async () => ({ profile }))
	} as unknown as Parameters<typeof load>[0]);
}

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

function vmcp(): VMCP {
	return {
		id: 'vmcp1-test',
		created: '2026-02-01T00:00:00Z',
		displayName: 'Virtual test server',
		description: 'Aggregates test tools',
		icon: 'https://example.com/vmcp.png',
		components: [
			{
				id: 'component-1',
				name: 'Component one',
				mcpCatalogID: 'default',
				mcpServerCatalogEntryID: 'entry-1',
				catalogEntry: { manifest: { name: 'Component one', runtime: 'npx' } }
			}
		],
		status: { ready: true }
	};
}

function vmcpInstance(): VMCPInstance {
	return {
		id: 'vmcpi1-test',
		created: '2026-02-02T00:00:00Z',
		userID: 'user-1',
		vmcpID: 'vmcp1-test',
		status: { configured: true }
	};
}

function vmcpRouteFetcher(target: VMCP, instances: VMCPInstance[] = []) {
	return vi.fn<typeof fetch>(async (input) => {
		const pathname = new URL(String(input)).pathname;
		if (pathname === `/api/vmcps/${target.id}`) return Response.json(target);
		if (pathname === '/api/vmcp-instances') return Response.json({ items: instances });
		const instance = instances.find(
			(candidate) => pathname === `/api/vmcp-instances/${candidate.id}`
		);
		if (instance) return Response.json(instance);
		return new Response('unexpected request', { status: 500 });
	});
}

describe('MCP tester route load', () => {
	it('derives a server-owned management route for Back navigation', async () => {
		const deployment = server();
		const result = await loadTester(deployment.id, routeFetcher([deployment]));

		expect(result).toMatchObject({
			server: { id: deployment.id, canConnect: true },
			backTarget: `/mcp-servers/c/${deployment.catalogEntryID}/instance/${deployment.id}`
		});
	});

	it('uses the existing 404 route convention for invalid or access-filtered IDs', async () => {
		await expect(loadTester('management-only-server', routeFetcher([]))).rejects.toMatchObject({
			status: 404
		});
	});

	it('loads a canonical vMCP and keeps its ID as the connection target', async () => {
		const target = vmcp();
		const result = await loadTester(target.id, vmcpRouteFetcher(target));

		expect(result).toMatchObject({
			server: {
				id: target.id,
				configured: true,
				deploymentStatus: 'Available',
				manifest: {
					name: target.displayName,
					description: target.description,
					runtime: 'vmcp'
				}
			},
			backTarget: `/vmcps/${target.id}`
		});
	});

	it('treats a canonical vMCP without an instance as setup-required when components need user configuration', async () => {
		const target = vmcp();
		target.components[0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
		const result = await loadTester(target.id, vmcpRouteFetcher(target));

		expect(result).toMatchObject({
			server: {
				id: target.id,
				configured: false,
				missingRequiredEnvVars: ['component-1.API_TOKEN']
			},
			backTarget: `/vmcps/${target.id}`
		});
	});

	it('attaches the current profile vMCP instance when loading a canonical vMCP', async () => {
		const target = vmcp();
		const otherUserInstance = {
			...vmcpInstance(),
			id: 'vmcpi1-other',
			userID: 'other-user',
			created: '2026-01-01T00:00:00Z',
			status: { configured: false, missingRequiredConfiguration: ['other.SECRET'] }
		};
		const currentUserInstance = {
			...vmcpInstance(),
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-1.API_KEY']
			}
		};
		const result = await loadTester(
			target.id,
			vmcpRouteFetcher(target, [otherUserInstance, currentUserInstance])
		);

		expect(result).toMatchObject({
			server: {
				id: target.id,
				userID: currentUserInstance.userID,
				configured: false,
				missingRequiredEnvVars: ['component-1.API_KEY'],
				manifest: { name: target.displayName, runtime: 'vmcp' }
			},
			backTarget: `/vmcps/${target.id}`
		});
	});

	it('loads a vMCP instance and reflects its configuration status', async () => {
		const target = vmcp();
		const instance = {
			...vmcpInstance(),
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-1.API_KEY']
			}
		};
		const result = await loadTester(instance.id, vmcpRouteFetcher(target, [instance]));

		expect(result).toMatchObject({
			server: {
				id: instance.id,
				userID: instance.userID,
				configured: false,
				missingRequiredEnvVars: ['component-1.API_KEY'],
				manifest: { name: target.displayName, runtime: 'vmcp' }
			},
			backTarget: `/vmcps/${target.id}`
		});
	});

	it('rejects a vMCP without components as non-connectable', async () => {
		const target = { ...vmcp(), components: [] };

		await expect(loadTester(target.id, vmcpRouteFetcher(target))).rejects.toMatchObject({
			status: 404
		});
	});
});
