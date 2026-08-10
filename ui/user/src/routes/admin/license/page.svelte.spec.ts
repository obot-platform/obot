import { COMMUNITY_ENTITLEMENT } from '$lib/constants';
import * as navigation from '$lib/navigation';
import type { License } from '$lib/services/admin/types';
import type { Version } from '$lib/services/user/types';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getLicenseResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import LicensePage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

async function renderLicensePage({
	license = getLicenseResponse,
	versionOverrides = {}
}: {
	license?: License;
	versionOverrides?: Partial<Version>;
} = {}) {
	const data = await preparePageData<PageData>({
		license,
		version: {
			...getVersionResponse,
			...versionOverrides
		}
	});

	return render(LicensePage, { data });
}

describe('Licensing Page', () => {
	beforeEach(() => {
		vi.mocked(navigation.reloadPage).mockImplementation(() => {});
	});

	afterEach(() => {
		vi.mocked(navigation.reloadPage).mockRestore();
	});

	it('renders community sign up when no license is present', async () => {
		await renderLicensePage({ license: getLicenseResponse });

		await expect
			.element(page.getByRole('heading', { name: 'Upgrade to Obot Community', exact: true }))
			.toBeVisible();
		await expect.element(page.getByLabelText('Name', { exact: true })).toBeVisible();
		await expect.element(page.getByLabelText('Email', { exact: true })).toBeVisible();
	});

	it('renders enterprise CTA when community license is present', async () => {
		await renderLicensePage({
			license: {
				...getLicenseResponse,
				licenseKey: 'community-license-key',
				enterprise: true,
				entitlements: [COMMUNITY_ENTITLEMENT]
			}
		});

		await expect
			.element(page.getByRole('heading', { name: 'Upgrade to Obot Enterprise', exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('link', { name: /Contact Us/i })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Upgrade to Obot Community', exact: true }))
			.not.toBeInTheDocument();
	});

	it('validating deleting an existing license', async () => {
		const deleteLicense = vi.fn(() => HttpResponse.json(getLicenseResponse));
		worker.use(http.delete('/api/license', deleteLicense));

		await renderLicensePage({
			license: {
				...getLicenseResponse,
				licenseKey: 'enterprise-license-key',
				enterprise: true,
				entitlements: ['OBOT_ENTERPRISE']
			}
		});

		await page.getByRole('button', { name: 'Delete License', exact: true }).click();
		await expect
			.element(page.getByText('Are you sure you want to delete the license?', { exact: true }))
			.toBeVisible();

		await page
			.getByRole('dialog')
			.getByRole('button', { name: 'Delete License', exact: true })
			.click();

		await vi.waitFor(() => {
			expect(deleteLicense).toHaveBeenCalledOnce();
			expect(navigation.reloadPage).toHaveBeenCalledOnce();
		});
	});

	it('renders banner when user limit is near limit', async () => {
		await renderLicensePage({
			versionOverrides: {
				userCount: 90,
				userLimit: 100
			}
		});

		await expect.element(page.getByText(/Contact us to upgrade to Obot Enterprise/i)).toBeVisible();
		await expect.element(page.getByText('90 / 100', { exact: true })).toBeVisible();
	});

	it('renders banner when user limit is reached', async () => {
		await renderLicensePage({
			versionOverrides: {
				userCount: 100,
				userLimit: 100,
				licenseEntitlementViolations: [
					{
						type: 'userLimit',
						namespace: 'default',
						name: 'users',
						requiredEntitlements: ['OBOT_ENTERPRISE'],
						missingEntitlements: ['OBOT_ENTERPRISE']
					}
				]
			}
		});

		await expect.element(page.getByText(/Contact us to upgrade to Obot Enterprise/i)).toBeVisible();
		await expect.element(page.getByText('100 / 100', { exact: true })).toBeVisible();
	});

	it('renders banner when auth provider is invalid', async () => {
		await renderLicensePage({
			versionOverrides: {
				licenseEntitlementViolations: [
					{
						type: 'authProvider',
						namespace: 'default',
						name: 'okta',
						requiredEntitlements: ['OBOT_ENTERPRISE'],
						missingEntitlements: ['OBOT_ENTERPRISE'],
						message: 'Auth provider requires an enterprise license'
					}
				]
			}
		});

		await expect.element(page.getByText(/Your license is/i)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Resolve', exact: true })).toBeVisible();
	});
});
