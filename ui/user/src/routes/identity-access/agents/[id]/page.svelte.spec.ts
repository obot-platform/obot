import { ApiKeysService, Group } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

const apiKey: APIKey = {
	id: 42,
	userId: 7,
	name: 'Test Agent Scope',
	canAccessAPI: false,
	canAccessLLMProxy: true,
	canAccessSkills: false,
	canAccessDeviceScans: false,
	createdAt: '2026-01-01T00:00:00.000Z'
};

function loadAgentAuthScope(groups: string[]) {
	const profile = createMockProfile(groups);
	return load({
		fetch: vi.fn(),
		params: { id: apiKey.id.toString() },
		parent: vi.fn(async () => ({ profile }))
	} as unknown as Parameters<typeof load>[0]);
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('Agent Auth Scope detail load', () => {
	it('gets the current-user scope for users without admin access', async () => {
		const getApiKey = vi.spyOn(ApiKeysService, 'getApiKey').mockResolvedValue(apiKey);
		const getAnyApiKey = vi.spyOn(ApiKeysService, 'getAnyApiKey');

		const result = await loadAgentAuthScope([Group.USER]);

		expect(result).toEqual({ apiKey, isAdmin: false });
		expect(getApiKey).toHaveBeenCalledWith(apiKey.id.toString(), expect.any(Object));
		expect(getAnyApiKey).not.toHaveBeenCalled();
	});

	it('gets any scope for users with admin access', async () => {
		const getApiKey = vi.spyOn(ApiKeysService, 'getApiKey');
		const getAnyApiKey = vi.spyOn(ApiKeysService, 'getAnyApiKey').mockResolvedValue(apiKey);

		const result = await loadAgentAuthScope([Group.ADMIN]);

		expect(result).toEqual({ apiKey, isAdmin: true });
		expect(getApiKey).not.toHaveBeenCalled();
		expect(getAnyApiKey).toHaveBeenCalledWith(apiKey.id.toString(), expect.any(Object));
	});
});
