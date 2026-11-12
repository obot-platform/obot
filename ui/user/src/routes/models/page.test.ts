import { AdminService, ApiKeysService, Group, type APIKey, type Model } from '$lib/services';
import { createMockProfile, createPageData } from '../../tests/helpers/pageData';
import { load as legacyAdminLoad } from '../admin/model-providers/+page';
import { load } from './+page';
import { isRedirect } from '@sveltejs/kit';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function loadModels(search: string, groups = [Group.ADMIN], models: Model[] = []) {
	return load({
		fetch: vi.fn(),
		parent: async () => createPageData({ profile: createMockProfile(groups), models }),
		url: new URL(`http://localhost/models${search}`)
	} as unknown as Parameters<NonNullable<typeof load>>[0]);
}

async function redirectOf(run: () => unknown) {
	try {
		await run();
	} catch (e) {
		if (isRedirect(e)) return { status: e.status, location: e.location };
		throw e;
	}
	throw new Error('load did not redirect');
}

const llmKey: APIKey = {
	id: 1,
	userId: 1,
	name: 'LLM Gateway Access Token',
	canAccessAPI: false,
	canAccessLLMProxy: true,
	canAccessSkills: false,
	canAccessDeviceScans: false,
	createdAt: '2026-01-01T00:00:00.000Z'
};

describe('Models page loader', () => {
	beforeEach(() => {
		vi.spyOn(ApiKeysService, 'listAllApiKeys').mockResolvedValue([]);
		vi.spyOn(ApiKeysService, 'listApiKeys').mockResolvedValue([]);
	});

	afterEach(() => vi.restoreAllMocks());

	it('sends the old model providers tab to the Providers tab, keeping other params', async () => {
		expect(await redirectOf(() => loadModels('?view=model-providers&query=open'))).toEqual({
			status: 307,
			location: '/models?view=providers&query=open'
		});
	});

	it('sends the old admin model providers page to the Providers tab', async () => {
		const location = (
			await redirectOf(() =>
				legacyAdminLoad({
					url: new URL('http://localhost/admin/model-providers')
				} as unknown as Parameters<typeof legacyAdminLoad>[0])
			)
		).location;
		expect(location.startsWith('/models?view=providers')).toBe(true);
	});

	it('sends the old access policies and AI judge policies tabs to the Profiles tab', async () => {
		for (const view of ['access-policies', 'ai-judge-policies']) {
			expect(await redirectOf(() => loadModels(`?view=${view}&new=true&query=x`))).toEqual({
				status: 307,
				location: '/models?view=profiles&query=x'
			});
		}
	});

	it('loads the gateway models a user can use on the Connect tab', async () => {
		const gatewayModel = {
			id: 'g1',
			active: true,
			modelProvider: 'llm-gateway-model-provider',
			targetModel: 'openai/gpt-4o'
		} as Model;
		const otherModel = { ...gatewayModel, id: 'o1', modelProvider: 'openai-model-provider' };

		for (const search of ['', '?view=connect', '?view=providers']) {
			const result = await loadModels(search, [Group.USER], [gatewayModel, otherModel]);
			expect(result).toMatchObject({ models: [gatewayModel], hasAdminAccess: false });
		}
	});

	it("loads the viewer's own identity API keys on the Connect tab, even for admins", async () => {
		vi.mocked(ApiKeysService.listApiKeys).mockResolvedValue([llmKey]);

		for (const groups of [[Group.ADMIN], [Group.USER]]) {
			vi.mocked(ApiKeysService.listApiKeys).mockClear();

			const result = await loadModels('?view=connect', groups);

			expect(ApiKeysService.listApiKeys).toHaveBeenCalledOnce();
			expect(result).toMatchObject({ apiKeys: [llmKey] });
		}
		expect(ApiKeysService.listAllApiKeys).not.toHaveBeenCalled();
	});

	it('still loads models when listing API keys fails', async () => {
		const gatewayModel = {
			id: 'g1',
			active: true,
			modelProvider: 'llm-gateway-model-provider',
			targetModel: 'openai/gpt-4o'
		} as Model;
		vi.mocked(ApiKeysService.listApiKeys).mockRejectedValue(new Error('boom'));

		const result = await loadModels('?view=connect', [Group.USER], [gatewayModel]);

		expect(result).toMatchObject({ models: [gatewayModel], apiKeys: [] });
	});

	it('does not load identity API keys off the Connect tab', async () => {
		await loadModels('?view=providers');

		expect(ApiKeysService.listAllApiKeys).not.toHaveBeenCalled();
		expect(ApiKeysService.listApiKeys).not.toHaveBeenCalled();
	});

	it('does not load model providers on the Providers tab', async () => {
		const list = vi.spyOn(AdminService, 'listModelProviders');

		const result = await loadModels('?view=providers');

		expect(list).not.toHaveBeenCalled();
		expect(result).toMatchObject({ hasAdminAccess: true });
	});

	it('does not load admin data for users on admin tabs', async () => {
		const list = vi.spyOn(AdminService, 'listModelAccessPolicies');

		const result = await loadModels('?view=profiles', [Group.USER]);

		expect(list).not.toHaveBeenCalled();
		expect(result).toMatchObject({ hasAdminAccess: false });
	});
});
