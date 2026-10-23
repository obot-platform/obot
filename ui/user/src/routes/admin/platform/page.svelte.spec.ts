import { page as appPage } from '$app/state';
import { CLOUD_ENTITLEMENT, COMMUNITY_ENTITLEMENT } from '$lib/constants';
import * as navigation from '$lib/navigation';
import { Group, type ModelProxySettings, type ProductTelemetryConsent } from '$lib/services';
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
	modelProxySettings
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
} = {}) {
	appPage.url.searchParams.set('view', view);
	if (create) {
		appPage.url.searchParams.set('create', 'true');
	} else {
		appPage.url.searchParams.delete('create');
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
					entitlements: { ...getLicenseResponse.entitlements, users: 100 },
					usage: { ...getLicenseResponse.usage, users: 90 }
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
					entitlements: { ...getLicenseResponse.entitlements, users: 100 },
					usage: { ...getLicenseResponse.usage, users: 100 }
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
		const billingLicense: License = {
			...getLicenseResponse,
			locked: true,
			entitlements: {
				...getLicenseResponse.entitlements,
				cloud: true,
				enterpriseAuthProviders: true,
				users: 10,
				devices: null,
				hostedMcpServers: 3,
				memoryMiB: 4096,
				cpuMillicores: 2000,
				auditLogRetentionDays: 7
			},
			usage: { users: 4, devices: 2, hostedMcpServers: 1, memoryMiB: 512, cpuMillicores: 250 },
			subscription: {
				plan: 'Team',
				status: 'active',
				periodEnd: '2026-11-01T00:00:00Z',
				cancelAtPeriodEnd: false,
				selfService: true
			}
		};

		const billing: Partial<Version> = { billingEnabled: true };

		afterEach(() => {
			appPage.url.searchParams.delete('billing');
		});

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
				.element(page.getByLabelText('CPU: 0.25 vCPU of 2 vCPU', { exact: true }))
				.toBeVisible();
			await expect.element(page.getByText('7 days', { exact: true })).toBeVisible();
			await expect.element(page.getByText(/on a plan with/)).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('heading', { name: 'Upgrade to Obot Enterprise', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Update License Key', exact: true }))
				.not.toBeInTheDocument();
		});

		it('lets owners manage billing on the billing pages', async () => {
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
			await expect.element(page.getByText(/Payment is past due/)).toBeVisible();
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
			appPage.url.searchParams.set('billing', 'returned');

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
					usage: { ...billingLicense.usage, hostedMcpServers: 4 }
				},
				versionOverrides: billing
			});

			await expect
				.element(page.getByText(/Hosted MCP servers is at 4 on a plan with 3\. Remove 1/))
				.toBeVisible();
			await expect.element(page.getByText('4 of 3 · over by 1', { exact: true })).toBeVisible();
		});

		it('shows why the environment is restricted', async () => {
			await renderPlatformPage({
				license: {
					...billingLicense,
					usage: { ...billingLicense.usage, hostedMcpServers: 4 }
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
				.element(page.getByText(/Restricted: Hosted MCP servers is at 4 on a plan with 3/))
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
