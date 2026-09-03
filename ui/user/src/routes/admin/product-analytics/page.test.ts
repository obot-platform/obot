import { Group } from '$lib/services';
import { createPageData } from '../../../tests/helpers/pageData';
import { load } from './+page';
import { describe, expect, it, vi } from 'vitest';

describe('Product Analytics settings loader', () => {
	it('returns 404 without rendering controls when consent is force-enabled', async () => {
		const parentData = createPageData({
			productTelemetryConsentAvailable: false,
			productTelemetryConsent: undefined
		});
		const fetch = vi.fn();

		await expect(
			load({
				fetch,
				parent: async () => parentData
			} as unknown as Parameters<NonNullable<typeof load>>[0])
		).rejects.toMatchObject({ status: 404 });
		expect(fetch).not.toHaveBeenCalled();
	});

	it('returns 403 for a non-administrator', async () => {
		const parentData = createPageData({
			profile: createPageData().profile,
			productTelemetryConsentAvailable: undefined
		});
		parentData.profile.groups = [Group.AUDITOR];

		await expect(
			load({
				fetch: vi.fn(),
				parent: async () => parentData
			} as unknown as Parameters<NonNullable<typeof load>>[0])
		).rejects.toMatchObject({ status: 403 });
	});
});
