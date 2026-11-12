import {
	trackProbe,
	trackRangeDataset,
	type Probe,
	type RangeDataset
} from './dashboardData.svelte';
import { flushSync } from 'svelte';
import { describe, expect, it, vi } from 'vitest';

type Deferred<T> = { resolve: (value: T) => void; reject: (error: unknown) => void };

function deferredFetch<T>() {
	const pending: Deferred<T>[] = [];
	const fn = vi.fn(
		() =>
			new Promise<T>((resolve, reject) => {
				pending.push({ resolve, reject });
			})
	);
	return { fn, pending };
}

const rangeA = { start: new Date(1000), end: new Date(2000) };
const rangeB = { start: new Date(3000), end: new Date(4000) };

describe('trackRangeDataset', () => {
	it('clears busy when a pending range is replaced by one it already has', async () => {
		let range = $state(rangeA);
		const { fn, pending } = deferredFetch<string>();
		let dataset: RangeDataset<string> | undefined;
		const destroy = $effect.root(() => {
			dataset = trackRangeDataset({ enabled: () => true, range: () => range, fetch: fn });
		});
		flushSync();

		pending[0].resolve('A');
		await vi.waitFor(() => expect(dataset?.loading).toBe(false));

		range = rangeB;
		flushSync();
		expect(dataset?.busy).toBe(true);

		range = rangeA;
		flushSync();
		expect(dataset?.busy).toBe(false);
		expect(dataset?.loading).toBe(false);
		expect(dataset?.data).toBe('A');
		expect(fn).toHaveBeenCalledTimes(2);
		destroy();
	});

	it('clears old data when a fetch fails and does not fetch while disabled', async () => {
		let range = $state(rangeA);
		let enabled = $state(true);
		const { fn, pending } = deferredFetch<string>();
		let dataset: RangeDataset<string> | undefined;
		const destroy = $effect.root(() => {
			dataset = trackRangeDataset({ enabled: () => enabled, range: () => range, fetch: fn });
		});
		flushSync();
		pending[0].resolve('A');
		await vi.waitFor(() => expect(dataset?.data).toBe('A'));

		range = rangeB;
		flushSync();
		pending[1].reject(new Error('boom'));
		await vi.waitFor(() => expect(dataset?.loading).toBe(false));
		expect(dataset?.data).toBeUndefined();

		enabled = false;
		range = rangeA;
		flushSync();
		expect(dataset?.loading).toBe(true);
		expect(fn).toHaveBeenCalledTimes(2);
		destroy();
	});
});

describe('trackProbe', () => {
	it('retries a failed check on the next retry key and keeps a success', async () => {
		let key = $state(0);
		const { fn, pending } = deferredFetch<boolean>();
		let probe: Probe | undefined;
		const destroy = $effect.root(() => {
			probe = trackProbe({ enabled: () => true, retryKey: () => key, check: fn });
		});
		flushSync();

		pending[0].reject(new Error('down'));
		await vi.waitFor(() => expect(probe?.settled).toBe(true));
		expect(probe?.value).toBe(false);

		key = 1;
		flushSync();
		pending[1].resolve(true);
		await vi.waitFor(() => expect(probe?.value).toBe(true));

		key = 2;
		flushSync();
		expect(fn).toHaveBeenCalledTimes(2);
		destroy();
	});

	it('aborts an in-flight check when destroyed', () => {
		let signal: AbortSignal | undefined;
		const destroy = $effect.root(() => {
			trackProbe({
				enabled: () => true,
				retryKey: () => 0,
				check: (s) => {
					signal = s;
					return new Promise<boolean>(() => {});
				}
			});
		});
		flushSync();
		expect(signal?.aborted).toBe(false);
		destroy();
		expect(signal?.aborted).toBe(true);
	});
});
