import { CommonAuthProviderIds } from '$lib/constants';
import { UserService, type AuthProvider } from '$lib/services';
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

describe('login hint', () => {
	it("passes an agent approval link's login hint on to the local login", async () => {
		vi.spyOn(UserService, 'getBootstrapStatus').mockResolvedValue({
			enabled: false,
			setupEnabled: false
		});
		vi.spyOn(UserService, 'listAuthProviders').mockResolvedValue([
			{ id: CommonAuthProviderIds.LOCAL } as AuthProvider
		]);
		const url = new URL('https://obot.example/');
		url.searchParams.set('rd', '/agents/approve/adr1req');
		url.searchParams.set('login_hint', 'ada@example.com');

		const result = load({
			fetch: vi.fn(),
			url,
			parent: vi.fn(async () => ({}))
		} as unknown as Parameters<typeof load>[0]);

		await expect(result).rejects.toMatchObject({
			status: 302,
			location:
				'/login/local?rd=' +
				encodeURIComponent('/agents/approve/adr1req') +
				'&login_hint=' +
				encodeURIComponent('ada@example.com')
		});
	});
});
