import { CommonAuthProviderIds } from '$lib/constants';
import { Group, UserService, type AuthProvider } from '$lib/services';
import { createMockProfile } from '../tests/helpers/pageData';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => vi.restoreAllMocks());

describe('local login redirect', () => {
	it.each(['', '/auth/device-code', '/mcp-servers?view=all&search=a%20b#details'])(
		'preserves the requested destination %j',
		async (rd) => {
			vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
				enabled: false,
				setupEnabled: false
			});
			vi.spyOn(UserService, 'listAuthProviders').mockResolvedValue([
				{ id: CommonAuthProviderIds.LOCAL } as AuthProvider
			]);
			const url = new URL('https://obot.example/');
			if (rd) url.searchParams.set('rd', rd);

			const result = load({
				fetch: vi.fn(),
				url,
				parent: vi.fn(async () => ({}))
			} as unknown as Parameters<typeof load>[0]);

			await expect(result).rejects.toMatchObject({
				status: 302,
				location: '/login/local?rd=' + encodeURIComponent(rd || '/')
			});
		}
	);
});

describe('signed-in landing page', () => {
	it.each([
		{ groups: [Group.ADMIN], destination: '/supervision' },
		{ groups: [Group.USER, Group.AUDITOR], destination: '/supervision' },
		{ groups: [Group.POWERUSER], destination: '/dashboard' },
		{ groups: [Group.USER], destination: '/vmcps' }
	])('sends $groups to $destination', async ({ groups, destination }) => {
		vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
			enabled: false,
			setupEnabled: false
		});
		await expect(
			load({
				fetch: vi.fn(),
				url: new URL('https://obot.example/'),
				parent: async () => ({ profile: createMockProfile(groups) })
			} as unknown as Parameters<typeof load>[0])
		).rejects.toMatchObject({ status: 302, location: destination });
	});
	it('preserves an explicit destination for administrators', async () => {
		vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
			enabled: false,
			setupEnabled: false
		});
		await expect(
			load({
				fetch: vi.fn(),
				url: new URL('https://obot.example/?rd=/sessions'),
				parent: async () => ({ profile: createMockProfile([Group.ADMIN]) })
			} as unknown as Parameters<typeof load>[0])
		).rejects.toMatchObject({ status: 302, location: '/sessions' });
	});
});
