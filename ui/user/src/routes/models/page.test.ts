import { AdminService, Group, type Model } from '$lib/services';
import { createMockProfile, createPageData } from '../../tests/helpers/pageData';
import { load as legacyAdminLoad } from '../admin/model-providers/+page';
import { load } from './+page';
import { isRedirect } from '@sveltejs/kit';
import { afterEach, describe, expect, it, vi } from 'vitest';

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

describe('Models page loader', () => {
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
