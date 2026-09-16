import { AdminService, Group, UserService } from '$lib/services';
import { createMockProfile } from '../../tests/helpers/pageData';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

function loadAdmin(pathname = '/admin') {
	return load({
		fetch: vi.fn(),
		url: new URL(pathname, 'http://localhost')
	} as unknown as Parameters<typeof load>[0]);
}

function ownerProfile() {
	return createMockProfile([Group.OWNER, Group.ADMIN]);
}

function bootstrapProfile() {
	const profile = ownerProfile();
	profile.username = 'bootstrap';
	profile.isBootstrapUser = () => true;
	return profile;
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('admin page load', () => {
	it('sends a bootstrap session to setup when setup is enabled', async () => {
		vi.spyOn(UserService, 'getProfile').mockResolvedValue(bootstrapProfile());
		const getBootstrapStatus = vi
			.spyOn(UserService, 'getBootstrapStatus')
			.mockResolvedValue({ enabled: true, setupEnabled: true });
		vi.spyOn(AdminService, 'listAllVMCPs').mockResolvedValue([]);

		await expect(loadAdmin()).rejects.toMatchObject({
			status: 307,
			location: '/admin/setup'
		});
		expect(getBootstrapStatus).toHaveBeenCalledOnce();
	});

	it('does not send a bootstrap session to setup when setup is disabled', async () => {
		vi.spyOn(UserService, 'getProfile').mockResolvedValue(bootstrapProfile());
		vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
			enabled: true,
			setupEnabled: false
		});
		vi.spyOn(AdminService, 'listAllVMCPs').mockResolvedValue([]);

		await expect(loadAdmin()).rejects.toMatchObject({
			status: 307,
			location: '/vmcps?new=true'
		});
	});

	it('sends a normal owner with no VMCPs to create a VMCP', async () => {
		vi.spyOn(UserService, 'getProfile').mockResolvedValue(ownerProfile());
		vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
			enabled: false,
			setupEnabled: false
		});
		vi.spyOn(AdminService, 'listAllVMCPs').mockResolvedValue([]);

		await expect(loadAdmin()).rejects.toMatchObject({
			status: 307,
			location: '/vmcps?new=true'
		});
	});
});
