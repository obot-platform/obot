import { page as appPage } from '$app/state';
import { CLOUD_ENTITLEMENT, COMMUNITY_ENTITLEMENT } from '$lib/constants';
import * as navigation from '$lib/navigation';
import type {
	ImagePullSecret,
	ImagePullSecretCapability,
	License
} from '$lib/services/admin/types';
import type { Version } from '$lib/services/user/types';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getLicenseResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import PlatformPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

async function renderPlatformPage({
	license = getLicenseResponse,
	versionOverrides = {},
	view = 'license',
	create = false,
	capability = { available: false },
	imagePullSecrets = []
}: {
	license?: License;
	versionOverrides?: Partial<Version>;
	view?: string;
	create?: boolean;
	capability?: ImagePullSecretCapability;
	imagePullSecrets?: ImagePullSecret[];
} = {}) {
	appPage.url.searchParams.set('view', view);
	if (create) {
		appPage.url.searchParams.set('create', 'true');
	} else {
		appPage.url.searchParams.delete('create');
	}

	const data = await preparePageData<PageData>({
		license,
		appNotification: undefined,
		k8sSettings: undefined,
		capability,
		imagePullSecrets,
		gitCredentials: [],
		version: {
			...getVersionResponse,
			...versionOverrides
		}
	});

	return render(PlatformPage, { data });
}

afterEach(() => {
	appPage.url.searchParams.delete('view');
	appPage.url.searchParams.delete('create');
	appPage.url.searchParams.delete('id');
});

describe('Platform Page', () => {
	beforeEach(() => {
		vi.mocked(navigation.reloadPage).mockImplementation(() => {});
	});

	afterEach(() => {
		vi.mocked(navigation.reloadPage).mockRestore();
	});

	describe('license tab', () => {
		it('renders enterprise CTA when no license is present', async () => {
			await renderPlatformPage({ license: getLicenseResponse });

			await expect
				.element(page.getByRole('button', { name: 'License', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Upgrade to Obot Enterprise', exact: true }))
				.toBeVisible();
			await expect.element(page.getByRole('link', { name: /Contact Us/i })).toBeVisible();
		});

		it('renders enterprise CTA when community license is present', async () => {
			await renderPlatformPage({
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
			await expect.element(page.getByLabelText('Name', { exact: true })).not.toBeInTheDocument();
		});

		it('shows open-source status and entitlements without delete for community license', async () => {
			await renderPlatformPage({
				license: {
					...getLicenseResponse,
					licenseKey: 'community-license-key',
					enterprise: true,
					entitlements: [COMMUNITY_ENTITLEMENT]
				}
			});

			await expect.element(page.getByText('License Status', { exact: true })).toBeVisible();
			await expect.element(page.getByText(/N\/A\s*\(Open-Source\)/)).toBeVisible();
			await expect.element(page.getByText('Entitlements', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Obot Community', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Delete License', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(page.getByText('Danger Zone', { exact: true })).not.toBeInTheDocument();
		});

		it('lets the user register for a community license when none is present', async () => {
			const createCommunityLicense = vi.fn();
			worker.use(
				http.post('/api/license/community', async ({ request }) => {
					createCommunityLicense(await request.json());
					return HttpResponse.json({
						...getLicenseResponse,
						licenseKey: 'community-license-key',
						enterprise: true,
						entitlements: [COMMUNITY_ENTITLEMENT]
					});
				})
			);

			await renderPlatformPage({ license: getLicenseResponse });
			await expect.element(page.getByLabelText('Name', { exact: true })).toBeVisible();
			await expect.element(page.getByLabelText('Email', { exact: true })).toBeVisible();
			await expect.element(page.getByLabelText('Company', { exact: false })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Delete License', exact: true }))
				.not.toBeInTheDocument();

			await page.getByLabelText('Name', { exact: true }).fill('Ada Lovelace');
			await page.getByLabelText('Email', { exact: true }).fill('ada@example.com');
			await page.getByLabelText('Company', { exact: false }).fill('Analytical Engine');
			await page.getByRole('button', { name: 'Register', exact: true }).click();

			await vi.waitFor(() => {
				expect(createCommunityLicense).toHaveBeenCalledWith({
					name: 'Ada Lovelace',
					email: 'ada@example.com',
					company: 'Analytical Engine'
				});
				expect(navigation.reloadPage).toHaveBeenCalledOnce();
			});
		});

		it('validating deleting an existing license', async () => {
			const deleteLicense = vi.fn(() => HttpResponse.json(getLicenseResponse));
			worker.use(http.delete('/api/license', deleteLicense));

			await renderPlatformPage({
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
			await renderPlatformPage({
				versionOverrides: {
					userCount: 90,
					userLimit: 100
				}
			});

			await expect
				.element(page.getByText(/Contact us to upgrade to Obot Enterprise/i))
				.toBeVisible();
			await expect.element(page.getByText('90 / 100', { exact: true })).toBeVisible();
		});

		it('renders banner when user limit is reached', async () => {
			await renderPlatformPage({
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

			await expect
				.element(page.getByText(/Contact us to upgrade to Obot Enterprise/i))
				.toBeVisible();
			await expect.element(page.getByText('100 / 100', { exact: true })).toBeVisible();
		});

		it('renders banner when auth provider is invalid', async () => {
			await renderPlatformPage({
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
			await expect
				.element(page.getByRole('button', { name: 'Resolve', exact: true }))
				.toBeVisible();
		});

		it('renders the Cloud entitlement label ahead of non-edition entitlements', async () => {
			await renderPlatformPage({
				license: {
					...getLicenseResponse,
					licenseKey: 'cloud-license-key',
					enterprise: true,
					entitlements: ['OBOT_SOME_FEATURE', CLOUD_ENTITLEMENT]
				}
			});

			await expect.element(page.getByText('Obot Cloud', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Some Feature', { exact: true })).toBeVisible();

			const badges = await page.getByRole('listitem').elements();
			const badgeText = badges.map((el) => el.textContent?.trim());
			expect(badgeText.indexOf('Obot Cloud')).toBeLessThan(badgeText.indexOf('Some Feature'));
		});
	});

	describe('registry connections tab', () => {
		it('hides the tab when the engine is not kubernetes', async () => {
			await renderPlatformPage({
				view: 'license',
				capability: { available: true }
			});

			await expect
				.element(page.getByRole('button', { name: 'Registry Connections', exact: true }))
				.not.toBeInTheDocument();
		});

		it('renders image pull secrets as the tab content', async () => {
			await renderPlatformPage({
				view: 'registry-connections',
				capability: { available: true },
				versionOverrides: { engine: 'kubernetes' }
			});

			await expect
				.element(page.getByRole('button', { name: 'Registry Connections', exact: true }))
				.toBeVisible();
			await expect.element(page.getByText('No image pull secrets', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Create New Secret', exact: true }).first())
				.toBeVisible();
		});

		it('opens the create form from the platform overlay', async () => {
			await renderPlatformPage({
				view: 'registry-connections',
				create: true,
				capability: { available: true },
				versionOverrides: { engine: 'kubernetes' }
			});

			await expect
				.element(page.getByRole('heading', { name: 'Create Image Pull Secret', exact: true }))
				.toBeVisible();
			await expect.element(page.getByText('Registry Server', { exact: true })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Create', exact: true })).toBeVisible();
		});
	});
});
