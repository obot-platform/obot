import { page as appPage } from '$app/state';
import { CLOUD_ENTITLEMENT, COMMUNITY_ENTITLEMENT, SEEN_SPLASH_DIALOG_KEY } from '$lib/constants';
import { markSeenTimestamp } from '$lib/localstate';
import * as navigation from '$lib/navigation';
import {
	Group,
	type ModelProxySettings,
	type RegistrySettings,
	type ProductTelemetryConsent
} from '$lib/services';
import type {
	ImagePullSecret,
	ImagePullSecretCapability,
	License
} from '$lib/services/admin/types';
import type { Version } from '$lib/services/user/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getLicenseResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import PlatformPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	setUrlParamAndUpdateUrl: vi.fn()
}));

async function renderPlatformPage({
	license = getLicenseResponse,
	versionOverrides = {},
	view = 'license',
	create = false,
	capability = { available: false },
	imagePullSecrets = [],
	groups = [Group.ADMIN],
	productAnalyticsAvailable = true,
	productAnalyticsConsent = { consent: false },
	modelProxySettings,
	registrySettings
}: {
	license?: License;
	versionOverrides?: Partial<Version>;
	view?: string;
	create?: boolean;
	capability?: ImagePullSecretCapability;
	imagePullSecrets?: ImagePullSecret[];
	groups?: string[];
	productAnalyticsAvailable?: boolean;
	productAnalyticsConsent?: ProductTelemetryConsent;
	modelProxySettings?: ModelProxySettings;
	registrySettings?: RegistrySettings;
} = {}) {
	(appPage.url as URL).searchParams.set('view', view);
	if (create) {
		(appPage.url as URL).searchParams.set('create', 'true');
	} else {
		(appPage.url as URL).searchParams.delete('create');
	}

	const data = await preparePageData<PageData>({
		profile: createMockProfile(groups),
		license,
		appNotification: undefined,
		k8sSettings: undefined,
		capability,
		imagePullSecrets,
		gitCredentials: [],
		productTelemetryConsentAvailable: productAnalyticsAvailable,
		productTelemetryConsent: productAnalyticsConsent,
		modelProxySettings,
		registrySettings,
		version: {
			...getVersionResponse,
			...versionOverrides
		}
	});

	return render(PlatformPage, { data });
}

afterEach(() => {
	(appPage.url as URL).searchParams.delete('view');
	(appPage.url as URL).searchParams.delete('create');
	(appPage.url as URL).searchParams.delete('id');
});

describe('Platform Page', () => {
	beforeEach(() => {
		vi.mocked(navigation.reloadPage).mockImplementation(() => {});
	});

	afterEach(() => {
		vi.mocked(navigation.reloadPage).mockRestore();
	});

	describe('settings tab', () => {
		it('shows product analytics inside Settings instead of its own tab', async () => {
			await renderPlatformPage({ view: 'settings' });

			await expect
				.element(page.getByRole('button', { name: 'Settings', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Product Analytics', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByText(/Share product usage data to help improve Obot\./))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Product Analytics', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Cancel', exact: true })).toBeVisible();
		});

		it('hides the product analytics section when consent controls are unavailable', async () => {
			await renderPlatformPage({ view: 'settings', productAnalyticsAvailable: false });

			await expect
				.element(page.getByRole('heading', { name: 'Product Analytics', exact: true }))
				.not.toBeInTheDocument();
		});

		it('hides product analytics and the shared actions from read-only administrators', async () => {
			await renderPlatformPage({ view: 'settings', groups: [Group.AUDITOR] });

			await expect
				.element(page.getByRole('heading', { name: 'Product Analytics', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Save', exact: true }))
				.not.toBeInTheDocument();
		});
	});

	describe('registry discovery settings', () => {
		const checkbox = () =>
			page.getByRole('checkbox', { name: 'Allow anonymous registry discovery' });
		const save = () => page.getByRole('button', { name: 'Save', exact: true });

		for (const engine of ['docker', 'kubernetes'] as const) {
			it(`shows the disabled setting and public metadata risks on ${engine}`, async () => {
				await renderPlatformPage({
					view: 'settings',
					versionOverrides: { engine },
					registrySettings: { anonymousAccessEnabled: false }
				});
				await expect.element(checkbox()).not.toBeChecked();
				await expect
					.element(page.getByText('Anonymous discovery publishes server metadata'))
					.toBeVisible();
				await expect.element(page.getByText(/full descriptions/)).toBeVisible();
				await expect
					.element(page.getByText(/Connecting still requires authentication/))
					.toBeVisible();
				await expect.element(save()).toBeDisabled();
			});
		}

		for (const group of [Group.ADMIN, Group.OWNER]) {
			it(`lets ${group} enable discovery with Save, then disable it`, async () => {
				// Test an onboarded owner so the first-login setup dialog does not block settings.
				markSeenTimestamp(SEEN_SPLASH_DIALOG_KEY);
				const update = vi.fn();
				worker.use(
					http.put('/api/registry-settings', async ({ request }) => {
						const body = await request.json();
						update(body);
						return HttpResponse.json(body);
					})
				);
				await renderPlatformPage({
					view: 'settings',
					// Owner profiles include the inherited Admin group.
					groups: group === Group.OWNER ? [Group.OWNER, Group.ADMIN] : [Group.ADMIN],
					registrySettings: { anonymousAccessEnabled: false }
				});
				await checkbox().click();
				expect(update).not.toHaveBeenCalled();
				await expect.element(save()).toBeEnabled();
				await save().click();
				await vi.waitFor(() =>
					expect(update).toHaveBeenCalledWith({ anonymousAccessEnabled: true })
				);
				await expect.element(save()).toBeDisabled();
				await expect.element(checkbox()).toBeChecked();
				await checkbox().click();
				await save().click();
				await vi.waitFor(() =>
					expect(update).toHaveBeenLastCalledWith({ anonymousAccessEnabled: false })
				);
				await expect.element(save()).toBeDisabled();
			});
		}

		it('cancels unsaved changes without writing settings', async () => {
			const update = vi.fn();
			worker.use(http.put('/api/registry-settings', update));
			await renderPlatformPage({
				view: 'settings',
				registrySettings: { anonymousAccessEnabled: true }
			});
			await checkbox().click();
			await page.getByRole('button', { name: 'Cancel', exact: true }).click();
			await expect.element(checkbox()).toBeChecked();
			await expect.element(save()).toBeDisabled();
			expect(update).not.toHaveBeenCalled();
		});

		it('hides registry discovery settings from auditors', async () => {
			await renderPlatformPage({
				view: 'settings',
				groups: [Group.AUDITOR],
				registrySettings: { anonymousAccessEnabled: true }
			});
			await expect
				.element(page.getByRole('heading', { name: 'MCP Registry API', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(checkbox()).not.toBeInTheDocument();
			await expect.element(save()).not.toBeInTheDocument();
		});

		it('shows unavailable settings rather than an editable disabled default', async () => {
			await renderPlatformPage({ view: 'settings' });
			await expect
				.element(
					page.getByText('Registry settings could not be loaded. Reload this page to try again.')
				)
				.toBeVisible();
			await expect.element(checkbox()).not.toBeInTheDocument();
		});

		it('retains dirty state after a failed save and allows retry', async () => {
			const update = vi.fn(() =>
				HttpResponse.json({ message: 'registry settings unavailable' }, { status: 503 })
			);
			worker.use(http.put('/api/registry-settings', update));
			await renderPlatformPage({
				view: 'settings',
				registrySettings: { anonymousAccessEnabled: false }
			});
			await checkbox().click();
			await save().click();
			await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
			await expect.element(save()).toBeEnabled();
			await expect.element(checkbox()).toBeChecked();
			worker.use(
				http.put('/api/registry-settings', () =>
					HttpResponse.json({ anonymousAccessEnabled: true })
				)
			);
			await save().click();
			await expect.element(save()).toBeDisabled();
		});
	});

	describe('license tab', () => {
		it('shows auditors only license information', async () => {
			await renderPlatformPage({ groups: [Group.AUDITOR] });

			await expect.element(page.getByText('License Status', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Entitlements', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Upgrade to Obot Enterprise', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(page.getByLabelText('Name', { exact: true })).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Add License Key', exact: true }))
				.not.toBeInTheDocument();
		});

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

	describe('registry connections section', () => {
		it('hides the section when the engine is not kubernetes', async () => {
			await renderPlatformPage({
				view: 'settings',
				capability: { available: true }
			});

			await expect
				.element(page.getByRole('heading', { name: 'Registry Connections', exact: true }))
				.not.toBeInTheDocument();
		});

		it('renders image pull secrets inside Settings', async () => {
			await renderPlatformPage({
				view: 'settings',
				capability: { available: true },
				versionOverrides: { engine: 'kubernetes' }
			});

			await expect
				.element(page.getByRole('button', { name: 'Settings', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Registry Connections', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByText("Click '+' to add a basic secret.", { exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Add Basic Secret', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Add ECR Secret', exact: true }))
				.toBeVisible();
		});

		it('opens the create form inside Settings', async () => {
			await renderPlatformPage({
				view: 'settings',
				create: true,
				capability: { available: true },
				versionOverrides: { engine: 'kubernetes' }
			});

			await expect
				.element(page.getByRole('button', { name: 'Settings', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Registry Connections', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Add Basic Secret', exact: true }))
				.toBeVisible();
			await expect.element(page.getByText('Registry Server', { exact: true })).toBeVisible();
			await expect
				.element(page.getByText("Click '+' to add a basic secret.", { exact: true }))
				.toBeVisible();
		});
	});
});
