import { UserService, type Profile, type VMCPInstance } from '$lib/services';
import { profile, vmcpInstances } from '$lib/stores';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const instance: VMCPInstance = {
	id: 'vmcpi-1',
	vmcpID: 'vmcp-1',
	userID: 'user-1',
	created: '2026-01-01T00:00:00Z'
};

describe('vmcpInstances', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		profile.current = {
			id: 'user-1',
			username: 'user-1',
			email: 'user@example.com',
			iconURL: '',
			role: 0,
			effectiveRole: 0,
			groups: [],
			loaded: true
		} as Profile;
		vmcpInstances.current = { items: [], loading: false };
	});

	it('loads the current user instances', async () => {
		vi.spyOn(UserService, 'listVMCPInstances').mockResolvedValue([instance]);

		await vmcpInstances.refresh();

		expect(vmcpInstances.current.items).toEqual([instance]);
		expect(vmcpInstances.current.loading).toBe(false);
	});

	it('replaces a saved instance without another list call', () => {
		vmcpInstances.current = { items: [instance], loading: false };

		const configured = {
			...instance,
			status: { configured: true }
		};
		vmcpInstances.upsert(configured);

		expect(vmcpInstances.current.items).toEqual([configured]);
	});

	it('refreshes when watching starts', async () => {
		vi.spyOn(UserService, 'listVMCPInstances').mockResolvedValue([instance]);

		const stop = vmcpInstances.startWatching();
		await vi.waitFor(() => expect(vmcpInstances.current.items).toEqual([instance]));
		stop();
	});
});
