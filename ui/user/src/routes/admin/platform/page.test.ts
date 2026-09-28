import { AdminService, Group, UserService } from '$lib/services';
import { createPageData, createMockProfile } from '../../../tests/helpers/pageData';
import { getAppNotificationResponse } from '../../../tests/mocks/data';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

function loadPlatform(
	view: string,
	{
		groups = [Group.ADMIN],
		available = true,
		consent = false
	}: { groups?: string[]; available?: boolean; consent?: boolean } = {}
) {
	return load({
		depends: vi.fn(),
		fetch: vi.fn(),
		parent: async () =>
			createPageData({
				profile: createMockProfile(groups),
				productTelemetryConsentAvailable: available,
				productTelemetryConsent: { consent }
			}),
		url: new URL(`http://localhost/admin/platform?view=${view}`)
	} as unknown as Parameters<NonNullable<typeof load>>[0]);
}

describe('Platform loader settings view', () => {
	afterEach(() => vi.restoreAllMocks());

	function mockSettingsApis() {
		vi.spyOn(UserService, 'getAppNotification').mockResolvedValue(getAppNotificationResponse);
		vi.spyOn(AdminService, 'getModelProxySettings').mockResolvedValue({
			enabled: false,
			url: 'https://model-service.obot.ai'
		});
		vi.spyOn(AdminService, 'getModelProxyUsage').mockResolvedValue({
			input: { max: 0, used: 0 },
			output: { max: 0, used: 0 },
			resetAt: '2026-01-01T00:00:00Z'
		});
		vi.spyOn(AdminService, 'listGitCredentials').mockResolvedValue([]);
	}

	it('fetches fresh consent when an administrator opens Settings', async () => {
		mockSettingsApis();
		const getConsent = vi
			.spyOn(AdminService, 'getProductTelemetryConsent')
			.mockResolvedValue({ consent: true });

		const result = await loadPlatform('settings', { consent: false });

		expect(getConsent).toHaveBeenCalledOnce();
		expect(result).toMatchObject({
			productTelemetryConsent: { consent: true },
			modelProxySettings: { enabled: false, url: 'https://model-service.obot.ai' }
		});
	});

	it.each([
		['consent controls are unavailable', [Group.ADMIN], false],
		['the user is a read-only administrator', [Group.AUDITOR], true]
	])('does not fetch consent when %s', async (_scenario, groups, available) => {
		mockSettingsApis();
		const getConsent = vi.spyOn(AdminService, 'getProductTelemetryConsent');

		await loadPlatform('settings', { groups, available });

		expect(getConsent).not.toHaveBeenCalled();
	});
});
