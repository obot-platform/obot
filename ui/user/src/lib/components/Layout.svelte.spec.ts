import {
	COMMUNITY_ENTITLEMENT,
	COMMUNITY_SIGNUP_BANNER_COPY,
	ENTERPRISE_ENTITLEMENT
} from '$lib/constants';
import { Group } from '$lib/services';
import type { License } from '$lib/services/admin/types';
import type { Profile, Version } from '$lib/services/user/types';
import { defaultModelAliases, license as licenseStore, profile, version } from '$lib/stores';
import { getLicenseResponse, getProfileResponse, getVersionResponse } from '../../tests/mocks/data';
import Layout from './Layout.svelte';
import { createRawSnippet, tick } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const children = createRawSnippet(() => ({ render: () => '<div></div>' }));

const sharedLinks = [
	'/dashboard',
	//'/vmcps',
	'/mcp-servers',
	'/skills',
	'/models',
	'/audit-logs',
	'/usage',
	'/inventory',
	'/identity-access'
];

const adminOnlyLinks = ['/admin/enforcement-events', '/admin/platform'];

function createProfile(groups: string[]): Profile {
	return {
		...getProfileResponse,
		groups,
		iconURL: '',
		loaded: true,
		canImpersonate: () => groups.includes(Group.ADMIN) && groups.includes(Group.USER_IMPERSONATION),
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
	profile.initialize({
		...createProfile(groups),
		...profileOverrides
	});
	version.initialize({
		...getVersionResponse,
		agentsEnabled: false,
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
	// await expandSection('ai-resources', '/vmcps');
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
				await expectAdminOnlyNavigation();
			});

			describe('when agents are enabled', () => {
				it('shows Launch Agent', async () => {
					await renderLayout([Group.ADMIN], { agentsEnabled: true });

					await expect.element(page.getByCSS('#launch-agent-chat')).toBeVisible();
				});
			});
		});

		describe('when the user is a power user', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER]);
				await expectSharedNavigation();
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a power user plus', () => {
			it('does not show administrator-only navigation', async () => {
				await renderLayout([Group.POWERUSER, Group.POWERUSER_PLUS]);
				await expectSharedNavigation();
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a basic user', () => {
			it('shows MCP Servers and does not show administrator-only navigation', async () => {
				await renderLayout([Group.USER]);
				await expectSharedNavigation();
				await expectLink('/mcp-servers');
				await expectNoAdminOnlyNavigation();
			});
		});

		describe('when the user is a basic user and auditor', () => {
			it('shows the administrator navigation available to auditors', async () => {
				await renderLayout([Group.USER, Group.AUDITOR]);
				await expectSharedNavigation();
				await expectAdminOnlyNavigation();
			});
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
