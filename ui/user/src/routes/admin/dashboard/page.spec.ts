import { UserService, type Profile } from '$lib/services';
import { load } from './+page';
import { beforeEach, describe, expect, it, vi } from 'vitest';

function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((resolvePromise) => {
		resolve = resolvePromise;
	});
	return { promise, resolve };
}

describe('admin dashboard page load', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it('starts the device scan request while parent data is still loading', async () => {
		const parentData = deferred<{ profile: Profile }>();
		const listDeviceScans = vi
			.spyOn(UserService, 'listDeviceScans')
			.mockResolvedValue({ items: [], total: 0, limit: 1, offset: 0 });

		const resultPromise = load({
			fetch,
			parent: () => parentData.promise
		} as Parameters<typeof load>[0]);

		expect(listDeviceScans).toHaveBeenCalledOnce();

		parentData.resolve({ profile: { unauthorized: false } as Profile });
		await expect(resultPromise).resolves.toEqual({ hasDeviceScans: false });
	});
});
