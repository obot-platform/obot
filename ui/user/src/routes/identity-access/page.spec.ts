import { ApiKeysService, Group, UserService } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile } from '../../tests/helpers/pageData';
import { listUsersResponse } from '../../tests/mocks/data';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

const apiKey: APIKey = {
	id: 42,
	userId: Number(listUsersResponse[0].id),
	name: 'Test Agent Scope',
	canAccessAPI: false,
	canAccessLLMProxy: true,
	canAccessSkills: false,
	canAccessDeviceScans: false,
	createdAt: '2026-01-01T00:00:00.000Z'
};

function loadIdentityAccess(pathname: string, groups: string[]) {
	const profile = createMockProfile(groups);
	return load({
		fetch: vi.fn(),
		parent: vi.fn(async () => ({ profile })),
		url: new URL(pathname, 'http://localhost')
	} as unknown as Parameters<typeof load>[0]);
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('Identity & Access load', () => {
	it('loads current-user scopes for non-admin agents view', async () => {
		const listApiKeys = vi.spyOn(ApiKeysService, 'listApiKeys').mockResolvedValue([apiKey]);
		const listAllApiKeys = vi.spyOn(ApiKeysService, 'listAllApiKeys');
		const listUsers = vi.spyOn(UserService, 'listUsers');

		const result = await loadIdentityAccess('/identity-access?view=users', [Group.USER]);

		expect(result).toMatchObject({ apiKeys: [apiKey], users: [] });
		expect(listApiKeys).toHaveBeenCalledOnce();
		expect(listAllApiKeys).not.toHaveBeenCalled();
		expect(listUsers).not.toHaveBeenCalled();
	});

	it('loads all scopes and users for admin agents view', async () => {
		const listApiKeys = vi.spyOn(ApiKeysService, 'listApiKeys');
		const listAllApiKeys = vi.spyOn(ApiKeysService, 'listAllApiKeys').mockResolvedValue([apiKey]);
		const listUsers = vi.spyOn(UserService, 'listUsers').mockResolvedValue(listUsersResponse);

		const result = await loadIdentityAccess('/identity-access?view=agents', [Group.ADMIN]);

		expect(result).toMatchObject({
			apiKeys: [apiKey],
			users: listUsersResponse
		});
		expect(listApiKeys).not.toHaveBeenCalled();
		expect(listAllApiKeys).toHaveBeenCalledOnce();
		expect(listUsers).toHaveBeenCalledOnce();
	});
});
