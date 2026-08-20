import { resolveGroups } from './operations';
import { describe, expect, it } from 'vitest';

interface FetchRecorder {
	fetch: typeof fetch;
	/** The `ids` parameter of every request, in dispatch order. */
	batches: string[][];
	peakConcurrency: number;
}

/**
 * Builds a fetch that answers the group resolution endpoint, naming every ID it is asked about
 * except those in `unknown`, and records how many requests overlap.
 */
function recordingFetch(options: { unknown?: string[]; delayMs?: number } = {}): FetchRecorder {
	const unknown = new Set(options.unknown ?? []);
	const recorder: Partial<FetchRecorder> = { batches: [], peakConcurrency: 0 };

	let inFlight = 0;

	recorder.fetch = (async (url: string) => {
		const ids = (new URL(url).searchParams.get('ids') ?? '').split(',').filter(Boolean);
		recorder.batches!.push(ids);

		inFlight++;
		recorder.peakConcurrency = Math.max(recorder.peakConcurrency!, inFlight);

		// Yielding lets the other workers start before this one settles, so the peak reflects real
		// overlap rather than the order the promises happen to resolve in.
		await new Promise((resolve) => setTimeout(resolve, options.delayMs ?? 5));
		inFlight--;

		return {
			ok: true,
			status: 200,
			json: async () => ({
				items: ids.filter((id) => !unknown.has(id)).map((id) => ({ id, name: `Group ${id}` })),
				source: 'cache',
				degraded: false
			})
		} as unknown as Response;
	}) as unknown as typeof fetch;

	return recorder as FetchRecorder;
}

function idsOfLength(n: number): string[] {
	return Array.from({ length: n }, (_, i) => `entra/${String(i).padStart(4, '0')}`);
}

describe('resolveGroups', () => {
	it('makes no request for an empty input', async () => {
		const recorder = recordingFetch();

		expect(await resolveGroups([], { fetch: recorder.fetch })).toEqual([]);
		expect(recorder.batches).toHaveLength(0);
	});

	it('sends a single request when the ids fit in one batch', async () => {
		const recorder = recordingFetch();

		const resolved = await resolveGroups(idsOfLength(100), { fetch: recorder.fetch });

		expect(recorder.batches).toHaveLength(1);
		expect(resolved).toHaveLength(100);
	});

	it('splits a larger input across batches', async () => {
		const recorder = recordingFetch();

		const resolved = await resolveGroups(idsOfLength(250), { fetch: recorder.fetch });

		// The server rejects a batch over its limit outright rather than truncating, so the chunking
		// is what keeps a large assignment set resolvable at all.
		expect(recorder.batches).toHaveLength(3);
		expect(recorder.batches.every((batch) => batch.length <= 100)).toBe(true);
		expect(resolved).toHaveLength(250);
	});

	it('bounds how many requests are in flight at once', async () => {
		const recorder = recordingFetch();

		// Twenty batches: dispatching them all at once would multiply through the server into far
		// more identity-provider calls than the directory will serve.
		await resolveGroups(idsOfLength(2000), { fetch: recorder.fetch });

		expect(recorder.batches).toHaveLength(20);
		expect(recorder.peakConcurrency).toBeGreaterThan(1);
		expect(recorder.peakConcurrency).toBeLessThanOrEqual(4);
	});

	it('returns groups in the order the ids were asked for', async () => {
		// Later batches answer faster than earlier ones, so anything that appended results as they
		// arrived would come back out of order.
		const recorder = recordingFetch();
		let call = 0;
		const slowFirst = (async (url: string) => {
			const delay = call++ === 0 ? 40 : 1;
			await new Promise((resolve) => setTimeout(resolve, delay));
			return recorder.fetch(url as unknown as URL);
		}) as unknown as typeof fetch;

		const ids = idsOfLength(250);
		const resolved = await resolveGroups(ids, { fetch: slowFirst });

		expect(resolved.map((group) => group.id)).toEqual(ids);
	});

	it('omits ids the server could not resolve', async () => {
		// The server names unresolvable ids after themselves, but a group can also be absent
		// entirely; the caller must not get a hole in the list.
		const recorder = recordingFetch({ unknown: ['entra/0005'] });

		const resolved = await resolveGroups(idsOfLength(10), { fetch: recorder.fetch });

		expect(resolved).toHaveLength(9);
		expect(resolved.some((group) => group.id === 'entra/0005')).toBe(false);
	});

	it('rejects when a batch fails', async () => {
		// The page load catches this so that a failed name lookup does not discard the assignments
		// themselves; resolveGroups itself must still report the failure rather than silently
		// returning a partial list.
		const failing = (async () => {
			throw new Error('network down');
		}) as unknown as typeof fetch;

		await expect(resolveGroups(idsOfLength(250), { fetch: failing })).rejects.toThrow();
	});
});
