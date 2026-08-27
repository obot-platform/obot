import { listMCPs } from './operations';
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
