import { AdminService, Group, UserService, type Version } from '$lib/services';
import { createPageData, createMockProfile } from '../../../tests/helpers/pageData';
import { getAppNotificationResponse, getVersionResponse } from '../../../tests/mocks/data';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

function loadPlatform(
	view: string,
	{
		groups = [Group.ADMIN],
		available = true,
		consent = false,
		version = {}
	}: { groups?: string[]; available?: boolean; consent?: boolean; version?: Partial<Version> } = {}
) {
	return load({
		depends: vi.fn(),
		fetch: vi.fn(),
		parent: async () =>
			createPageData({
				profile: createMockProfile(groups),
				productTelemetryConsentAvailable: available,
				productTelemetryConsent: { consent },
				version: { ...getVersionResponse, ...version }
			}),
		url: new URL(`http://localhost/admin/platform?view=${view}`)
	} as unknown as Parameters<NonNullable<typeof load>>[0]);
}

describe('Platform loader settings view', () => {
	afterEach(() => vi.restoreAllMocks());

	function mockSettingsApis({ modelProxyEnabled = false }: { modelProxyEnabled?: boolean } = {}) {
		vi.spyOn(AdminService, 'getRegistrySettings').mockResolvedValue({
			anonymousAccessEnabled: false
		});
		vi.spyOn(UserService, 'getAppNotification').mockResolvedValue(getAppNotificationResponse);
		vi.spyOn(AdminService, 'getModelProxySettings').mockResolvedValue({
			enabled: modelProxyEnabled,
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

	it('fetches model proxy usage when the model proxy can be used', async () => {
		mockSettingsApis({ modelProxyEnabled: true });
		vi.spyOn(AdminService, 'getProductTelemetryConsent').mockResolvedValue({ consent: false });

		await loadPlatform('settings', {
			version: { hasModelProvider: false, hasValidLicense: true }
		});

		expect(AdminService.getModelProxyUsage).toHaveBeenCalledOnce();
	});

	it.each([
		['the model proxy is disabled', false, { hasModelProvider: false, hasValidLicense: true }],
		['a model provider is configured', true, { hasModelProvider: true, hasValidLicense: true }],
		[
			'model provider configuration is unknown',
			true,
			{ hasModelProvider: null, hasValidLicense: true }
		],
		['the installation is unlicensed', true, { hasModelProvider: false, hasValidLicense: false }]
	])('does not fetch model proxy usage when %s', async (_scenario, modelProxyEnabled, version) => {
		mockSettingsApis({ modelProxyEnabled });
		vi.spyOn(AdminService, 'getProductTelemetryConsent').mockResolvedValue({ consent: false });

		const result = await loadPlatform('settings', { version });

		expect(AdminService.getModelProxyUsage).not.toHaveBeenCalled();
		expect(result).toHaveProperty('modelProxyUsage', undefined);
	});
});

describe('Platform registry settings loader', () => {
	afterEach(() => vi.restoreAllMocks());
	function mockOtherSettingsApis() {
		vi.spyOn(UserService, 'getAppNotification').mockResolvedValue(getAppNotificationResponse);
		vi.spyOn(AdminService, 'getModelProxySettings').mockResolvedValue({ enabled: false, url: '' });
		vi.spyOn(AdminService, 'getModelProxyUsage').mockRejectedValue(new Error('usage unavailable'));
		vi.spyOn(AdminService, 'listGitCredentials').mockResolvedValue([]);
	}
	it.each([Group.ADMIN, Group.OWNER])('loads saved registry settings for %s', async (group) => {
		mockOtherSettingsApis();
		vi.spyOn(AdminService, 'getRegistrySettings').mockResolvedValue({
			anonymousAccessEnabled: true
		});
		const result = await loadPlatform('settings', { groups: [group], available: false });
		expect(result).toMatchObject({ registrySettings: { anonymousAccessEnabled: true } });
	});
	it('does not request registry settings for auditors', async () => {
		mockOtherSettingsApis();
		const getSettings = vi.spyOn(AdminService, 'getRegistrySettings');
		const result = await loadPlatform('settings', { groups: [Group.AUDITOR], available: false });
		expect(getSettings).not.toHaveBeenCalled();
		expect(result).toMatchObject({ registrySettings: undefined });
	});
	it('keeps failed registry settings unavailable rather than defaulting to disabled', async () => {
		mockOtherSettingsApis();
		vi.spyOn(AdminService, 'getRegistrySettings').mockRejectedValue(
			new Error('settings unavailable')
		);
		const result = await loadPlatform('settings', { available: false });
		expect(result).toMatchObject({ registrySettings: undefined });
	});
});
