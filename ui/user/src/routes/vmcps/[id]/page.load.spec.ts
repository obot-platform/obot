import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { getVersionResponse } from '../../../tests/mocks/data';
import { version as versionStore } from '$lib/stores';
import { load } from './+page';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub'
});
const vmcp = createVMCP({ id: 'vmcp-1', displayName: 'Issue Tracker vMCP' }, [componentEntry]);

function loadPage(id: string, fetcher: typeof fetch) {
	const profile = createMockProfile();
	return load({
		params: { id },
		fetch: fetcher,
		parent: vi.fn(async () => ({ profile }))
	} as unknown as Parameters<typeof load>[0]);
}

function routeFetcher(options?: { version?: Response; versionBody?: unknown }) {
	return vi.fn<typeof fetch>(async (input) => {
		const pathname = new URL(String(input)).pathname;
		if (pathname === `/api/vmcps/${vmcp.id}`) return Response.json(vmcp);
		if (pathname === '/api/users') return Response.json({ items: [] });
		if (pathname === '/api/version') {
			if (options?.version) return options.version;
			return Response.json(options?.versionBody ?? getVersionResponse);
		}
		return new Response('unexpected request', { status: 500 });
	});
}

describe('vMCP detail route load', () => {
	beforeEach(() => {
		versionStore.initialize();
	});

	it('refreshes version so tester chat availability can be re-evaluated', async () => {
		const version = { ...getVersionResponse, mcpTesterModelProxyAvailable: true };
		const result = await loadPage(vmcp.id, routeFetcher({ versionBody: version }));

		expect(result).toEqual({ vmcp, users: [] });
		expect(versionStore.current).toEqual(version);
	});

	it('still loads the vMCP when version cannot be refreshed', async () => {
		const result = await loadPage(
			vmcp.id,
			routeFetcher({ version: new Response('unavailable', { status: 500 }) })
		);

		expect(result).toEqual({ vmcp, users: [] });
		expect(versionStore.current).toEqual({});
	});
});
