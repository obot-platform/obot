import { SCIM_VIEW_PATH } from '$lib/constants';
import { HttpError } from '$lib/errors';
import { AdminService, ApiKeysService, Group, UserService } from '$lib/services';
import type {
	SCIMConnection,
	SCIMConnectionReview,
	SCIMEnablePreview
} from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile } from '../../tests/helpers/pageData';
import { listUsersResponse } from '../../tests/mocks/data';
import { load } from './+page';
import { isRedirect } from '@sveltejs/kit';
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

	describe('Auth Providers tab', () => {
		it('loads SCIM instead of the auth providers on the SCIM sub-tab', async () => {
			const review = { connection: { id: 'conn-1' } } as SCIMConnectionReview;
			vi.spyOn(AdminService, 'listSCIMConnections').mockResolvedValue([
				{ id: 'conn-1' } as SCIMConnection
			]);
			const getReview = vi.spyOn(AdminService, 'getSCIMConnectionReview').mockResolvedValue(review);
			const listAuthProviders = vi.spyOn(AdminService, 'listAuthProviders');

			const result = await loadIdentityAccess(SCIM_VIEW_PATH, [Group.OWNER, Group.ADMIN]);

			expect(result).toMatchObject({ scimReview: review, authProviders: [] });
			expect(getReview).toHaveBeenCalledWith('conn-1', expect.objectContaining({ limit: 50 }));
			expect(listAuthProviders).not.toHaveBeenCalled();
		});

		it('loads whether SCIM can be enabled when there is no connection', async () => {
			const preview = { blockers: [], duplicateGroupNames: [] } as SCIMEnablePreview;
			vi.spyOn(AdminService, 'listSCIMConnections').mockResolvedValue([]);
			vi.spyOn(AdminService, 'getSCIMEnablePreview').mockResolvedValue(preview);

			const result = await loadIdentityAccess(SCIM_VIEW_PATH, [Group.OWNER, Group.ADMIN]);

			expect(result).toMatchObject({ scimEnablePreview: preview, scimReview: undefined });
		});

		it('returns a signed-out user to the SCIM sub-tab after signing in', async () => {
			vi.spyOn(AdminService, 'listSCIMConnections').mockRejectedValue(
				new HttpError(401, 'unauthorized')
			);

			let redirect: unknown;
			try {
				await loadIdentityAccess(SCIM_VIEW_PATH, [Group.OWNER, Group.ADMIN]);
			} catch (err) {
				redirect = err;
			}

			expect(isRedirect(redirect)).toBe(true);
			const location = new URL((redirect as { location: string }).location, 'http://localhost');
			expect(location.searchParams.get('rd')).toBe(SCIM_VIEW_PATH);
		});

		it('loads the auth providers, and not SCIM, on the Providers sub-tab', async () => {
			vi.spyOn(UserService, 'getVersion').mockResolvedValue({ authEnabled: true } as Awaited<
				ReturnType<typeof UserService.getVersion>
			>);
			const listAuthProviders = vi.spyOn(AdminService, 'listAuthProviders').mockResolvedValue([]);
			const listSCIMConnections = vi.spyOn(AdminService, 'listSCIMConnections');

			const result = await loadIdentityAccess('/identity-access?view=auth-providers', [
				Group.OWNER,
				Group.ADMIN
			]);

			expect(result).toMatchObject({ authEnabled: true, scimReview: undefined });
			expect(listAuthProviders).toHaveBeenCalledOnce();
			expect(listSCIMConnections).not.toHaveBeenCalled();
		});
	});
});
