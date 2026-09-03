import { listMCPCatalogEntries } from './operations';
import { describe, expect, it, vi } from 'vitest';

describe('listMCPCatalogEntries', () => {
	it('combines all and minimal list options', async () => {
		const fetcher = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ items: [] }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			})
		);

		await listMCPCatalogEntries('default', { fetch: fetcher, all: true, minimal: true });

		expect(fetcher).toHaveBeenCalledOnce();
		expect(fetcher.mock.calls[0][0]).toMatch(
			/\/api\/mcp-catalogs\/default\/entries\?all=true&minimal=true$/
		);
	});
});
