import { UserService, type AuthProvider, type Profile } from '$lib/services';
import { createMockProfile } from '../../tests/helpers/pageData';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

const anonymous: Profile = {
	id: '',
	email: '',
	iconURL: '',
	role: 0,
	effectiveRole: 0,
	groups: [],
	unauthorized: true,
	username: ''
};

function loadActivate(profile: Profile, providers: Partial<AuthProvider>[]) {
	const listAuthProviders = vi
		.spyOn(UserService, 'listAuthProviders')
		.mockResolvedValue(providers as AuthProvider[]);
	const result = load({
		fetch: vi.fn(),
		parent: vi.fn(async () => ({ profile }))
	} as unknown as Parameters<typeof load>[0]);
	return { result, listAuthProviders };
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('activation page load', () => {
	it('serves the page to a visitor while activation is required', async () => {
		const { result } = loadActivate(anonymous, [{ requiresActivation: true }]);

		await expect(result).resolves.toBeUndefined();
	});

	it('goes home when nothing requires activation', async () => {
		const { result } = loadActivate(anonymous, [{ requiresActivation: false }]);

		await expect(result).rejects.toMatchObject({ status: 302, location: '/' });
	});

	it('goes home for a signed-in user', async () => {
		const { result } = loadActivate(createMockProfile(), [{ requiresActivation: true }]);

		await expect(result).rejects.toMatchObject({ status: 302, location: '/' });
	});

	it('leaves a setup session to the page without listing providers', async () => {
		const { result, listAuthProviders } = loadActivate(
			{ ...createMockProfile(), requirePasswordChange: true },
			[]
		);

		await expect(result).resolves.toBeUndefined();
		expect(listAuthProviders).not.toHaveBeenCalled();
	});
});
