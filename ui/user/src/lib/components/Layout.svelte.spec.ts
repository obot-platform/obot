import { page as appPage } from '$app/state';
import { m } from '$lib/i18n';
import { overwriteGetLocale } from '$lib/paraglide/runtime';
import { Group } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import type { Profile, Version } from '$lib/services/user/types';
import { profile, userDeviceSettings, version } from '$lib/stores';
import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
import { getProfileResponse, getVersionResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import Layout from './Layout.svelte';
import { http, HttpResponse } from 'msw';
import { createRawSnippet, tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

overwriteGetLocale(() => 'en');
const children = createRawSnippet(() => ({ render: () => '<div></div>' }));

const sharedLinks = ['/vmcps', '/skills', '/audit-logs', '/usage', '/identity-access'];

const adminOnlyLinks = ['/inventory', '/admin/platform'];

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
	await expandSection('operations', '/inventory');

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
	describe('removed model navigation', () => {
		it('does not expose the LLM Gateway models page', async () => {
			await renderLayout([Group.ADMIN]);
			await expandSection('ai-resources', '/vmcps');
			await expectNoLink('/models');
		});
	});

	describe('removed policy navigation', () => {
		it('does not expose message-policy or policy-violation routes', async () => {
			await renderLayout([Group.ADMIN]);

			await expandSection('operations', '/audit-logs');
			await expectNoLink('/admin/message-policies');
			await expectNoLink('/admin/policy-violations');
		});
	});

	describe('guardrails navigation', () => {
		it('has no standalone Guardrails entry', async () => {
			await renderLayout([Group.ADMIN]);
			await expandSection('ai-resources', '/vmcps');
			await expectNoLink('/guardrails');
		});

		it('no longer has a separate LLM Gateway section', async () => {
			await renderLayout([Group.ADMIN]);
			await expect.element(page.getByCSS('#sidebar-collapse-llm-gateway')).not.toBeInTheDocument();
			for (const href of [
				'/llm-gateway/overview',
				'/llm-gateway/providers',
				'/llm-gateway/budgets',
				'/llm-gateway/guardrails',
				'/llm-gateway/routing',
				'/llm-gateway/logs',
				'/llm-gateway/settings'
			]) {
				await expectNoLink(href);
			}
		});
	});

	describe('based on user role', () => {
		describe('when the user is an administrator', () => {
			it('shows administrator-only navigation', async () => {
				await renderLayout([Group.ADMIN]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/market');
				await expectAdminOnlyNavigation();
			});
		});

		describe('when the user is a power user', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/market');
				await expectLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a power user plus', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER, Group.POWERUSER_PLUS]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/market');
				await expectLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a basic user', () => {
			it('shows only the marketplace', async () => {
				await renderLayout([Group.USER]);
				await expectLink('/market');
				for (const href of [
					'/dashboard',
					'/vmcps',
					'/mcp-servers',
					'/skills',
					'/audit-logs',
					'/identity-access'
				]) {
					await expectNoLink(href);
				}
			});
		});

		describe('when the user is a basic user and auditor', () => {
			it('shows the administrator navigation available to auditors', async () => {
				await renderLayout([Group.USER, Group.AUDITOR]);
				await expectSharedNavigation();
				await expectLink('/dashboard');
				await expectLink('/market');
				await expectAdminOnlyNavigation();
			});
		});
	});

	describe('SCIM setup banner', () => {
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
			return renderLayout(groups, {}, profileOverrides);
		}

		const continueLink = () =>
			page.getByRole('link', { name: m.identity_access_scim_continue_setup(), exact: true });

		it('sends Owners to the SCIM tab until SCIM is enforced', async () => {
			await renderWithAuthProviders([okta()]);

			await expect.element(continueLink()).toBeVisible();
			await expect
				.element(continueLink())
				.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
			await expect
				.element(page.getByText(m.identity_access_scim_setup_banner({ provider: 'Okta' })))
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

		it('shows when no other banner has higher priority', async () => {
			worker.use(http.get('/api/auth-providers', () => HttpResponse.json({ items: [okta()] })));
			await adminConfigStore.refresh();
			await renderLayout([Group.OWNER, Group.ADMIN], {}, owner);

			await expect.element(continueLink()).toBeVisible();
		});
	});

	describe('SCIM token banner', () => {
		const day = 24 * 60 * 60 * 1000;
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
			return renderLayout(groups, {}, profileOverrides);
		}

		const rotateLink = () =>
			page.getByRole('link', { name: m.identity_access_scim_rotate_token_action(), exact: true });
		const dismissButton = () =>
			page.getByRole('button', {
				name: m.identity_access_scim_dismiss_token_banner(),
				exact: true
			});

		it('warns Owners within 30 days of the token expiring, and links to the SCIM tab', async () => {
			const expiring = oktaWithTokenExpiringIn(10 * day);
			await renderWithAuthProviders([expiring]);

			await expect
				.element(
					page.getByText(
						m.identity_access_scim_token_expiring_banner({
							provider: 'Okta',
							date: new Date(expiring.scimTokenExpiresAt as string).toLocaleDateString()
						})
					)
				)
				.toBeVisible();
			await expect
				.element(rotateLink())
				.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
			await expect.element(dismissButton()).toBeVisible();
		});

		it('says that provisioning fails once the token has expired, until it is rotated', async () => {
			const expired = oktaWithTokenExpiringIn(-day);
			await renderWithAuthProviders([expired]);

			await expect
				.element(
					page.getByRole('alert').filter({
						hasText: m.identity_access_scim_token_expired_banner({
							provider: 'Okta',
							date: new Date(expired.scimTokenExpiresAt as string).toLocaleDateString()
						})
					})
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
});
