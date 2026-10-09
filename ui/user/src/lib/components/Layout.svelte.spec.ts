import { page as appPage } from '$app/state';
import {
	COMMUNITY_ENTITLEMENT,
	COMMUNITY_SIGNUP_BANNER_COPY,
	ENTERPRISE_ENTITLEMENT
} from '$lib/constants';
import { Group } from '$lib/services';
import type { AuthProvider, License } from '$lib/services/admin/types';
import type { Profile, Version } from '$lib/services/user/types';
import {
	defaultModelAliases,
	license as licenseStore,
	profile,
	userDeviceSettings,
	version
} from '$lib/stores';
import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
import { getLicenseResponse, getProfileResponse, getVersionResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import Layout from './Layout.svelte';
import { http, HttpResponse } from 'msw';
import { createRawSnippet, tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const children = createRawSnippet(() => ({ render: () => '<div></div>' }));

const sharedLinks = ['/vmcps', '/skills', '/models', '/audit-logs', '/usage', '/identity-access'];

const adminOnlyLinks = ['/admin/enforcement-events', '/admin/platform'];

function createProfile(groups: string[]): Profile {
	return {
		...getProfileResponse,
		groups,
		iconURL: '',
		loaded: true,
		hasAdminAccess: () => groups.includes(Group.ADMIN) || groups.includes(Group.AUDITOR),
		isAdmin: () => groups.includes(Group.ADMIN),
		isAdminReadonly: () => !groups.includes(Group.ADMIN) && groups.includes(Group.AUDITOR),
		isBootstrapUser: () => false
	};
}

async function renderLayout(
	groups: string[] = [],
	versionOverrides: Partial<Version> = {},
	licenseOverrides: Partial<License> = {},
	profileOverrides: Partial<Profile> = {}
) {
	userDeviceSettings.setShowAllGuides(false);
	profile.initialize({
		...createProfile(groups),
		...profileOverrides
	});
	version.initialize({
		...getVersionResponse,
		engine: 'docker',
		...versionOverrides
	});
	licenseStore.initialize({
		...getLicenseResponse,
		...licenseOverrides
	});
	await defaultModelAliases.initialize([]);

	return render(Layout, { children });
}

async function clickButton(id: string) {
	const locator = page.getByCSS(`#${id}`);
	await expect.element(locator).toBeInTheDocument();
	// Native DOM click: Playwright actionability fails on driver.js overlays / off-viewport sidebar.
	const el = await locator.element();
	if (!(el instanceof HTMLElement)) {
		throw new Error(`Expected #${id} to be an HTMLElement`);
	}
	el.click();
	await tick();
}

async function expandSection(id: string, expectedHref: string) {
	const link = page.getByCSS(`a.sidebar-link[href="${expectedHref}"]`);
	if ((await link.elements()).length === 0) {
		await clickButton(`sidebar-collapse-${id}`);
	}
}

async function expectLink(href: string) {
	await expect.element(page.getByCSS(`a.sidebar-link[href="${href}"]`)).toBeInTheDocument();
}

async function expectNoLink(href: string) {
	await expect.element(page.getByCSS(`a.sidebar-link[href="${href}"]`)).not.toBeInTheDocument();
}

async function expectSharedNavigation() {
	await expandSection('ai-resources', '/vmcps');
	await expandSection('operations', '/audit-logs');

	for (const href of sharedLinks) {
		await expectLink(href);
	}
}

async function expectAdminOnlyNavigation() {
	await expandSection('operations', '/admin/enforcement-events');

	for (const href of adminOnlyLinks) {
		await expectLink(href);
	}
}

async function expectNoAdminOnlyNavigation() {
	for (const href of adminOnlyLinks) {
		await expectNoLink(href);
	}
}

describe('Layout.svelte', () => {
	it('gives all users access to shared sidebar navigation', async () => {
		await renderLayout();
		await expectSharedNavigation();
		await expectNoLink('/dashboard');
		await expectNoAdminOnlyNavigation();
	});

	describe('when Hosted Agents are disabled', () => {
		it('hides Hosted Agents navigation', async () => {
			await renderLayout([Group.ADMIN], { hostedAgentsEnabled: false });

			await expectNoLink('/hosted-agents');
		});
	});

	describe('when Hosted Agents are enabled', () => {
		it('shows Hosted Agents navigation', async () => {
			await renderLayout([Group.ADMIN], { hostedAgentsEnabled: true });
			await expectLink('/hosted-agents');
		});
	});

	describe('based on user role', () => {
		describe('when the user is an administrator', () => {
			it('shows administrator-only navigation', async () => {
				await renderLayout([Group.ADMIN]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectAdminOnlyNavigation();
				await expectNoLink('/admin/product-analytics');
			});
		});

		describe('when the user is a power user', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a power user plus', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER, Group.POWERUSER_PLUS]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a basic user', () => {
			it('hides MCP Servers and does not show administrator-only navigation', async () => {
				await renderLayout([Group.USER]);
				await expectSharedNavigation();
				await expectNoLink('/dashboard');
				await expectNoLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a basic user and auditor', () => {
			it('shows the administrator navigation available to auditors', async () => {
				await renderLayout([Group.USER, Group.AUDITOR]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectAdminOnlyNavigation();
				await expectNoLink('/admin/product-analytics');
			});
		});
	});

	describe('SCIM setup banner', () => {
		const communityLicense: Partial<License> = {
			licenseKey: 'community-license-key',
			enterprise: true,
			entitlements: [COMMUNITY_ENTITLEMENT]
		};
		const owner: Partial<Profile> = {
			isOwner: () => true
		};

		function okta(overrides: Partial<AuthProvider> = {}): AuthProvider {
			return {
				id: 'okta-auth-provider',
				created: '2026-09-01T00:00:00.000Z',
				type: 'authprovider',
				name: 'Okta',
				image: '',
				port: 0,
				configured: true,
				scimState: 'connected',
				...overrides
			};
		}

		async function renderWithAuthProviders(
			authProviders: AuthProvider[],
			groups: string[] = [Group.OWNER, Group.ADMIN],
			profileOverrides: Partial<Profile> = owner
		) {
			worker.use(
				http.get('/api/auth-providers', () => HttpResponse.json({ items: authProviders }))
			);
			await adminConfigStore.refresh();
			return renderLayout(groups, {}, communityLicense, profileOverrides);
		}

		const continueLink = () => page.getByRole('link', { name: 'Continue SCIM setup', exact: true });

		it('sends Owners to the SCIM tab until SCIM is enforced', async () => {
			await renderWithAuthProviders([okta()]);

			await expect.element(continueLink()).toBeVisible();
			await expect
				.element(continueLink())
				.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
			await expect
				.element(page.getByText(/Okta provisions users and groups through SCIM/))
				.toBeVisible();
		});

		it('does not show once SCIM is enforced, or without SCIM', async () => {
			await renderWithAuthProviders([okta({ scimState: 'enforced' })]);
			await expect.element(continueLink()).not.toBeInTheDocument();

			await renderWithAuthProviders([okta({ scimState: undefined })]);
			await expect.element(continueLink()).not.toBeInTheDocument();
		});

		it('does not show for a provider that is not configured', async () => {
			await renderWithAuthProviders([okta({ configured: false })]);

			await expect.element(continueLink()).not.toBeInTheDocument();
		});

		it('does not show for administrators who are not Owners, or for the bootstrap user', async () => {
			await renderWithAuthProviders([okta()], [Group.ADMIN], {});
			await expect.element(continueLink()).not.toBeInTheDocument();

			await renderWithAuthProviders([okta()], [Group.OWNER, Group.ADMIN], {
				...owner,
				isBootstrapUser: () => true
			});
			await expect.element(continueLink()).not.toBeInTheDocument();
		});

		it('does not show on the SCIM tab it links to', async () => {
			const url = vi
				.spyOn(appPage, 'url', 'get')
				.mockReturnValue(
					new URL(
						'http://localhost/identity-access?view=auth-providers&subview=scim'
					) as typeof appPage.url
				);
			try {
				await renderWithAuthProviders([okta()]);
				await expect.element(continueLink()).not.toBeInTheDocument();
			} finally {
				url.mockRestore();
			}
		});

		it('cannot be dismissed, even on a device where it was dismissed before', async () => {
			localStorage.setItem('@obot/dismiss-scim-setup-banner', 'true');
			await renderWithAuthProviders([okta()]);

			await expect.element(continueLink()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Dismiss SCIM setup banner', exact: true }))
				.not.toBeInTheDocument();
		});

		it('shows instead of the community signup banner', async () => {
			worker.use(http.get('/api/auth-providers', () => HttpResponse.json({ items: [okta()] })));
			await adminConfigStore.refresh();
			await renderLayout([Group.OWNER, Group.ADMIN], {}, {}, owner);

			await expect.element(continueLink()).toBeVisible();
			await expect
				.element(page.getByText(COMMUNITY_SIGNUP_BANNER_COPY, { exact: true }))
				.not.toBeInTheDocument();
		});
	});

	describe('SCIM token banner', () => {
		const day = 24 * 60 * 60 * 1000;
		const communityLicense: Partial<License> = {
			licenseKey: 'community-license-key',
			enterprise: true,
			entitlements: [COMMUNITY_ENTITLEMENT]
		};
		const owner: Partial<Profile> = {
			isOwner: () => true
		};

		function oktaWithTokenExpiringIn(
			ms: number,
			overrides: Partial<AuthProvider> = {}
		): AuthProvider {
			return {
				id: 'okta-auth-provider',
				created: '2026-09-01T00:00:00.000Z',
				type: 'authprovider',
				name: 'Okta',
				image: '',
				port: 0,
				configured: true,
				scimState: 'enforced',
				scimTokenExpiresAt: new Date(Date.now() + ms).toISOString(),
				...overrides
			};
		}

		async function renderWithAuthProviders(
			authProviders: AuthProvider[],
			groups: string[] = [Group.OWNER, Group.ADMIN],
			profileOverrides: Partial<Profile> = owner
		) {
			worker.use(
				http.get('/api/auth-providers', () => HttpResponse.json({ items: authProviders }))
			);
			await adminConfigStore.refresh();
			return renderLayout(groups, {}, communityLicense, profileOverrides);
		}

		const rotateLink = () => page.getByRole('link', { name: 'Rotate SCIM token', exact: true });
		const dismissButton = () =>
			page.getByRole('button', { name: 'Dismiss SCIM token banner', exact: true });

		it('warns Owners within 30 days of the token expiring, and links to the SCIM tab', async () => {
			await renderWithAuthProviders([oktaWithTokenExpiringIn(10 * day)]);

			await expect.element(page.getByText(/The SCIM token for Okta expires on/)).toBeVisible();
			await expect
				.element(rotateLink())
				.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
			await expect.element(dismissButton()).toBeVisible();
		});

		it('says that provisioning fails once the token has expired, until it is rotated', async () => {
			await renderWithAuthProviders([oktaWithTokenExpiringIn(-day)]);

			await expect
				.element(
					page.getByRole('alert').filter({ hasText: /expired on .*provisioning from Okta fails/ })
				)
				.toBeVisible();
			await expect.element(rotateLink()).toBeVisible();
			await expect.element(dismissButton()).not.toBeInTheDocument();
		});

		it('does not show while the token is far from expiring', async () => {
			await renderWithAuthProviders([oktaWithTokenExpiringIn(200 * day)]);

			await expect.element(rotateLink()).not.toBeInTheDocument();
		});

		it('does not show for a provider that is not configured', async () => {
			await renderWithAuthProviders([oktaWithTokenExpiringIn(-day, { configured: false })]);

			await expect.element(rotateLink()).not.toBeInTheDocument();
		});

		it('does not show for administrators who are not Owners, or for the bootstrap user', async () => {
			await renderWithAuthProviders([oktaWithTokenExpiringIn(-day)], [Group.ADMIN], {});
			await expect.element(rotateLink()).not.toBeInTheDocument();

			await renderWithAuthProviders([oktaWithTokenExpiringIn(-day)], [Group.OWNER, Group.ADMIN], {
				...owner,
				isBootstrapUser: () => true
			});
			await expect.element(rotateLink()).not.toBeInTheDocument();
		});

		it('does not show on the SCIM tab, which warns itself', async () => {
			const url = vi
				.spyOn(appPage, 'url', 'get')
				.mockReturnValue(
					new URL(
						'http://localhost/identity-access?view=auth-providers&subview=scim'
					) as typeof appPage.url
				);
			try {
				await renderWithAuthProviders([oktaWithTokenExpiringIn(-day)]);
				await expect.element(rotateLink()).not.toBeInTheDocument();
			} finally {
				url.mockRestore();
			}
		});

		it('stays dismissed until a token with another expiry is issued', async () => {
			const expiring = oktaWithTokenExpiringIn(10 * day);
			await renderWithAuthProviders([expiring]);

			// Native DOM click: Playwright actionability fails on driver.js overlays.
			const el = await dismissButton().element();
			if (!(el instanceof HTMLElement)) {
				throw new Error('Expected dismiss control to be an HTMLElement');
			}
			el.click();
			await expect.element(rotateLink()).not.toBeInTheDocument();

			await renderWithAuthProviders([expiring]);
			await expect.element(rotateLink()).not.toBeInTheDocument();

			// Every layout rendered so far reads the same providers, so each shows the new token's banner.
			await renderWithAuthProviders([oktaWithTokenExpiringIn(20 * day)]);
			await expect.element(rotateLink().first()).toBeVisible();
		});
	});

	describe('community signup banner', () => {
		const copy = COMMUNITY_SIGNUP_BANNER_COPY;

		it('shows for administrators without a community or enterprise license', async () => {
			await renderLayout([Group.ADMIN]);

			await expect.element(page.getByText(copy, { exact: true })).toBeVisible();
			const register = page.getByRole('link', { name: 'Register', exact: true });
			await expect.element(register).toBeVisible();
			await expect.element(register).toHaveAttribute('href', '/admin/platform?view=license');
		});

		it('does not show for basic users', async () => {
			await renderLayout([Group.USER]);

			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('does not show for auditors', async () => {
			await renderLayout([Group.USER, Group.AUDITOR]);

			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('does not show license actions for auditors with violations', async () => {
			await renderLayout([Group.USER, Group.AUDITOR], {
				licenseEntitlementViolations: [
					{
						type: 'userLimit',
						namespace: 'default',
						name: 'users',
						requiredEntitlements: [ENTERPRISE_ENTITLEMENT],
						missingEntitlements: [ENTERPRISE_ENTITLEMENT]
					}
				]
			});

			await expect
				.element(page.getByRole('button', { name: 'Resolve', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(page.getByText(/Upgrade to Obot Enterprise/)).not.toBeInTheDocument();
		});

		it('does not show when a community license is present', async () => {
			await renderLayout(
				[Group.ADMIN],
				{},
				{
					licenseKey: 'community-license-key',
					enterprise: true,
					entitlements: [COMMUNITY_ENTITLEMENT]
				}
			);

			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('does not show when an enterprise license is present', async () => {
			await renderLayout(
				[Group.ADMIN],
				{ enterprise: true },
				{
					licenseKey: 'enterprise-license-key',
					enterprise: true,
					entitlements: [ENTERPRISE_ENTITLEMENT]
				}
			);

			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('can be dismissed for this device', async () => {
			await renderLayout([Group.ADMIN]);

			const dismiss = page.getByRole('button', {
				name: 'Dismiss community signup banner',
				exact: true
			});
			await expect.element(dismiss).toBeVisible();
			// Native DOM click: Playwright actionability fails on driver.js overlays.
			const el = await dismiss.element();
			if (!(el instanceof HTMLElement)) {
				throw new Error('Expected dismiss control to be an HTMLElement');
			}
			el.click();
			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();

			await renderLayout([Group.ADMIN]);
			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('stays dismissed when dismissed after the profile was created', async () => {
			localStorage.setItem(
				'@obot/dismiss-community-signup-banner',
				JSON.stringify({ dismissedAt: '2026-08-10T00:00:00.000Z' })
			);

			await renderLayout([Group.ADMIN], {}, {}, { created: '2026-08-04T16:58:40.000Z' });

			await expect.element(page.getByText(copy, { exact: true })).not.toBeInTheDocument();
		});

		it('shows again when the profile was created after the banner was dismissed', async () => {
			localStorage.setItem(
				'@obot/dismiss-community-signup-banner',
				JSON.stringify({ dismissedAt: '2020-01-01T00:00:00.000Z' })
			);

			await renderLayout([Group.ADMIN], {}, {}, { created: '2026-08-04T16:58:40.000Z' });

			await expect.element(page.getByText(copy, { exact: true })).toBeVisible();
		});
	});
});
