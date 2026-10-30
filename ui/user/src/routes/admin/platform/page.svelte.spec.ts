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
import { errors } from '$lib/stores';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getLicenseResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import PlatformPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest';
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

			await expect.element(page.getByText('License key', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Open source', { exact: true })).toBeVisible();
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
					entitlements: { ...getLicenseResponse.entitlements, community: true }
				},
				versionOverrides: { hasValidLicense: true, licenseEntitlements: [COMMUNITY_ENTITLEMENT] }
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
					entitlements: { ...getLicenseResponse.entitlements, community: true }
				},
				versionOverrides: { hasValidLicense: true, licenseEntitlements: [COMMUNITY_ENTITLEMENT] }
			});

			await expect.element(page.getByText('Obot Community', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Community', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Delete License', exact: true }))
				.not.toBeInTheDocument();
		});

		it('lets the user register for a community license when none is present', async () => {
			const createCommunityLicense = vi.fn();
			worker.use(
				http.post('/api/license/community', async ({ request }) => {
					createCommunityLicense(await request.json());
					return HttpResponse.json({
						...getLicenseResponse,
						licenseKey: 'community-license-key',
						entitlements: { ...getLicenseResponse.entitlements, community: true }
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
					entitlements: { ...getLicenseResponse.entitlements, enterprise: true }
				},
				versionOverrides: { hasValidLicense: true, licenseEntitlements: ['OBOT_ENTERPRISE'] }
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
				license: {
					...getLicenseResponse,
					entitlements: { ...getLicenseResponse.entitlements, users: 100 }
				},
				versionOverrides: {
					userCount: 90,
					userLimit: 100
				}
			});

			await expect
				.element(page.getByText(/Contact us to upgrade to Obot Enterprise/i))
				.toBeVisible();
			await expect.element(page.getByLabelText('Users: 90 of 100', { exact: true })).toBeVisible();
		});

		it('renders banner when user limit is reached', async () => {
			await renderPlatformPage({
				license: {
					...getLicenseResponse,
					entitlements: { ...getLicenseResponse.entitlements, users: 100 }
				},
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
			await expect.element(page.getByLabelText('Users: 100 of 100', { exact: true })).toBeVisible();
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

		it('shows a Cloud license as the Cloud edition', async () => {
			await renderPlatformPage({
				license: {
					...getLicenseResponse,
					licenseKey: 'cloud-license-key',
					enterprise: true,
					entitlements: { ...getLicenseResponse.entitlements, cloud: true }
				},
				versionOverrides: {
					hasValidLicense: true,
					licenseEntitlements: [CLOUD_ENTITLEMENT]
				}
			});

			await expect.element(page.getByText('Obot Cloud', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Cloud', { exact: true })).toBeVisible();
			await expect.element(page.getByText('Valid', { exact: true })).toBeVisible();
		});
	});

	describe('billing', () => {
		const billingLicense = {
			...getLicenseResponse,
			locked: true,
			entitlements: {
				...getLicenseResponse.entitlements,
				cloud: true,
				enterpriseAuthProviders: true,
				users: 10,
				devices: null,
				memoryMiB: 4096,
				cpuMillicores: 2000,
				auditLogRetentionDays: 7
			},
			usage: { users: 4, devices: 2, memoryMiB: 512, cpuMillicores: 250 },
			subscription: {
				plan: 'Team',
				status: 'active',
				periodEnd: '2026-11-01T00:00:00Z',
				cancelAtPeriodEnd: false,
				selfService: true
			}
		} satisfies License;

		const billing: Partial<Version> = { billingEnabled: true };

		// A real tab would load the test page, so the browser blocks new tabs unless a test opens one.
		beforeEach(() => {
			vi.spyOn(window, 'open').mockReturnValue(null);
		});

		afterEach(() => {
			(appPage.url as URL).searchParams.delete('billing');
			vi.mocked(window.open).mockRestore();
		});

		// testTab stands in for the tab the manage button opens.
		function testTab() {
			return {
				opener: window as unknown,
				location: { assign: vi.fn() },
				close: vi.fn()
			};
		}

		it("shows the plan and its limits instead of the license key's controls", async () => {
			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});

			await expect.element(page.getByText('Team', { exact: true })).toBeVisible();
			await expect.element(page.getByText('License key', { exact: true })).not.toBeInTheDocument();
			await expect.element(page.getByLabelText('Users: 4 of 10', { exact: true })).toBeVisible();
			await expect
				.element(page.getByLabelText('Devices: 2, unlimited', { exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByLabelText('Memory: 512 MiB of 4 GiB', { exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByLabelText('CPU: 0.25 cores of 2 cores', { exact: true }))
				.toBeVisible();
			await expect.element(page.getByText('7 days', { exact: true })).toBeVisible();
			await expect.element(page.getByText(/and the plan includes/)).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('heading', { name: 'Upgrade to Obot Enterprise', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Update License Key', exact: true }))
				.not.toBeInTheDocument();
		});

		it('opens the billing pages in a new tab', async () => {
			worker.use(
				http.post('/api/license/manage', () =>
					HttpResponse.json(
						{ url: 'https://billing.example.com/session', expiresAt: '2026-10-20T12:05:00Z' },
						{ status: 201 }
					)
				)
			);
			const tab = testTab();
			vi.mocked(window.open).mockReturnValue(tab as unknown as Window);

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});
			const manageButton = page.getByRole('button', { name: 'Manage Billing', exact: true });
			await expect.element(manageButton).toBeEnabled();
			// Playwright's actionability checks don't settle on this button, so it's clicked natively.
			(manageButton.element() as HTMLElement).click();

			await vi.waitFor(() =>
				expect(tab.location.assign).toHaveBeenCalledWith('https://billing.example.com/session')
			);
			expect(window.open).toHaveBeenCalledWith('', '_blank');
			expect(tab.opener).toBeNull();
			expect(tab.close).not.toHaveBeenCalled();
			await expect.element(manageButton).toBeEnabled();
		});

		it("closes the new tab when billing won't open the billing pages", async () => {
			worker.use(
				http.post('/api/license/manage', () =>
					HttpResponse.json({ error: 'This environment is closed.' }, { status: 409 })
				)
			);
			const tab = testTab();
			vi.mocked(window.open).mockReturnValue(tab as unknown as Window);

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});
			const manageButton = page.getByRole('button', { name: 'Manage Billing', exact: true });
			await expect.element(manageButton).toBeEnabled();
			// Playwright's actionability checks don't settle on this button, so it's clicked natively.
			(manageButton.element() as HTMLElement).click();

			await vi.waitFor(() => expect(tab.close).toHaveBeenCalledOnce());
			expect(tab.location.assign).not.toHaveBeenCalled();
		});

		it('opens the billing pages here when the browser blocks the new tab', async () => {
			const manage = vi.fn();
			worker.use(
				http.post('/api/license/manage', () => {
					manage();
					// A same-page link, so following it doesn't navigate the test's frame away.
					return HttpResponse.json(
						{ url: '#billing-session', expiresAt: '2026-10-20T12:05:00Z' },
						{ status: 201 }
					);
				})
			);
			onTestFinished(() =>
				history.replaceState(null, '', window.location.pathname + window.location.search)
			);

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});
			const manageButton = page.getByRole('button', { name: 'Manage Billing', exact: true });
			await expect.element(manageButton).toBeVisible();
			await expect.element(manageButton).toBeEnabled();
			// Playwright's actionability checks don't settle on this button, so it's clicked natively.
			(manageButton.element() as HTMLElement).click();

			await vi.waitFor(() => {
				expect(manage).toHaveBeenCalledOnce();
				expect(window.location.hash).toBe('#billing-session');
			});
		});

		it("doesn't offer billing management to admins who aren't owners", async () => {
			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN],
				versionOverrides: billing
			});

			await expect.element(page.getByText('Team', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Manage Billing', exact: true }))
				.not.toBeInTheDocument();
		});

		it("doesn't offer billing management for a plan the account team manages", async () => {
			await renderPlatformPage({
				license: {
					...billingLicense,
					subscription: { ...billingLicense.subscription!, selfService: false }
				},
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});

			await expect
				.element(page.getByText(/This plan is managed by your account team\./))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Manage Billing', exact: true }))
				.not.toBeInTheDocument();
		});

		it("shows billing's message when it won't open the billing pages", async () => {
			worker.use(
				http.post('/api/license/manage', () =>
					HttpResponse.json({ error: 'This environment is closed.' }, { status: 409 })
				)
			);
			errors.items = [];

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});
			const manageButton = page.getByRole('button', { name: 'Manage Billing', exact: true });
			await expect.element(manageButton).toBeVisible();
			// Playwright's actionability checks don't settle on this button, so it's clicked natively.
			(manageButton.element() as HTMLElement).click();

			await vi.waitFor(() => expect(errors.items).toHaveLength(1));
			expect(errors.items[0].message).toBe(
				'Failed to open the billing pages: This environment is closed.'
			);
		});

		it('shows auditors the plan without the actions that change it', async () => {
			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.AUDITOR],
				versionOverrides: billing
			});

			await expect.element(page.getByText('Team', { exact: true })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Check Now', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Manage Billing', exact: true }))
				.not.toBeInTheDocument();
		});

		it('says when payment is past due', async () => {
			await renderPlatformPage({
				license: {
					...billingLicense,
					subscription: { ...billingLicense.subscription!, status: 'pastDue' }
				},
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});

			await expect.element(page.getByText(/^Past Due/)).toBeVisible();
			await expect.element(page.getByText(/A payment didn't go through/)).toBeVisible();
		});

		it("shows billing's layout before billing first answers", async () => {
			await renderPlatformPage({
				license: { ...billingLicense, subscription: undefined },
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});

			await expect.element(page.getByText('Waiting for billing', { exact: true })).toBeVisible();
			await expect
				.element(page.getByText(/hasn't confirmed this environment's subscription/))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Check Now', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Add License Key', exact: true }))
				.not.toBeInTheDocument();
		});

		it('checks the subscription when an owner returns from billing', async () => {
			const check = vi.fn();
			worker.use(
				http.post('/api/license', () => {
					check();
					return HttpResponse.json({
						...billingLicense,
						subscription: { ...billingLicense.subscription!, plan: 'Business' }
					});
				})
			);
			worker.use(
				http.get('/api/version', () => HttpResponse.json({ ...getVersionResponse, ...billing }))
			);
			(appPage.url as URL).searchParams.set('billing', 'returned');

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});

			await vi.waitFor(() => expect(check).toHaveBeenCalledOnce());
			await expect.element(page.getByText('Business', { exact: true })).toBeVisible();
		});

		it('waits for the sync a check requests before showing the plan', async () => {
			worker.use(
				http.post('/api/license', () => HttpResponse.json({ ...billingLicense, syncing: true })),
				http.get('/api/license', () =>
					HttpResponse.json({
						...billingLicense,
						subscription: { ...billingLicense.subscription!, plan: 'Business' }
					})
				),
				http.get('/api/version', () => HttpResponse.json({ ...getVersionResponse, ...billing }))
			);

			await renderPlatformPage({
				license: billingLicense,
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: billing
			});
			const checkButton = page.getByRole('button', { name: 'Check Now', exact: true });
			await expect.element(checkButton).toBeEnabled();
			// Playwright's actionability checks don't settle on this button, so it's clicked natively.
			(checkButton.element() as HTMLElement).click();

			await expect.element(page.getByText('Business', { exact: true })).toBeVisible();
		});

		it('warns while the environment is over its limits', async () => {
			await renderPlatformPage({
				license: {
					...billingLicense,
					usage: { ...billingLicense.usage, memoryMiB: 5120 }
				},
				versionOverrides: billing
			});

			await expect
				.element(page.getByText(/Memory is at 5 GiB, and the plan includes 4 GiB\. Free up 1 GiB/))
				.toBeVisible();
			await expect
				.element(page.getByText('5 GiB of 4 GiB · over by 1 GiB', { exact: true }))
				.toBeVisible();
		});

		it('shows why the environment is restricted', async () => {
			await renderPlatformPage({
				license: {
					...billingLicense,
					usage: { ...billingLicense.usage, memoryMiB: 5120 }
				},
				groups: [Group.ADMIN, Group.OWNER],
				versionOverrides: {
					...billing,
					licenseEntitlementViolations: [
						{
							type: 'restricted',
							namespace: '',
							name: '',
							requiredEntitlements: [],
							missingEntitlements: []
						}
					]
				}
			});

			await expect
				.element(
					page.getByText(
						/Only admins can sign in right now: Memory is at 5 GiB, and the plan includes 4 GiB/
					)
				)
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Change plan', exact: true }))
				.toBeVisible();
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
