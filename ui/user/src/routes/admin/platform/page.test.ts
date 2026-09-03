import { AdminService, Group, UserService } from '$lib/services';
import { createPageData, createMockProfile } from '../../../tests/helpers/pageData';
import { getLicenseResponse } from '../../../tests/mocks/data';
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

describe('Platform loader product analytics view', () => {
	afterEach(() => vi.restoreAllMocks());

	it('fetches fresh consent when an administrator selects Product Analytics', async () => {
		const getConsent = vi
			.spyOn(AdminService, 'getProductTelemetryConsent')
			.mockResolvedValue({ consent: true });

		const result = await loadPlatform('product-analytics', { consent: false });

		expect(getConsent).toHaveBeenCalledOnce();
		expect(result).toMatchObject({ productTelemetryConsent: { consent: true } });
	});

	it.each([
		['consent controls are unavailable', [Group.ADMIN], false],
		['the user is a read-only administrator', [Group.AUDITOR], true]
	])('does not fetch consent when %s', async (_scenario, groups, available) => {
		const getConsent = vi.spyOn(AdminService, 'getProductTelemetryConsent');
		vi.spyOn(UserService, 'getLicense').mockResolvedValue(getLicenseResponse);

		await loadPlatform('product-analytics', { groups, available });

		expect(getConsent).not.toHaveBeenCalled();
	});
});
