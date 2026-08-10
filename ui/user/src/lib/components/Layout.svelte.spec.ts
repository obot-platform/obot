import { Group } from '$lib/services';
import type { Profile, Version } from '$lib/services/user/types';
import { defaultModelAliases, profile, version } from '$lib/stores';
import { getProfileResponse, getVersionResponse } from '../../tests/mocks/data';
import Layout from './Layout.svelte';
import { createRawSnippet, tick } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const children = createRawSnippet(() => ({ render: () => '<div></div>' }));

const adminSections = [
	{ id: 'mcp-server-management', href: '/admin/mcp-catalog' },
	{ id: 'skills-management', href: '/admin/skills' },
	{ id: 'device-management', href: '/admin/devices' },
	{ id: 'user-management', href: '/admin/users' },
	{ id: 'llm-gateway', href: '/admin/token-usage' },
	{ id: 'app-management', href: '/admin/license' }
];

const adminSectionLabels = [
	'MCP Management',
	'Skills Management',
	'Device Management',
	'Auth Management',
	'LLM Gateway',
	'App Management'
];

const adminSharedLinks = [
	'/admin/mcp-catalog',
	'/admin/mcp-access-policies',
	'/admin/mcp-deployments',
	'/admin/audit-logs',
	'/admin/usage',
	'/admin/filters',
	'/admin/skills',
	'/admin/skill-access-policies',
	'/admin/devices',
	'/admin/enforcement-decisions',
	'/admin/users',
	'/admin/groups',
	'/admin/user-roles',
	'/admin/auth-providers',
	'/admin/agent-auth-scopes',
	'/admin/token-usage',
	'/admin/llm-audit-logs',
	'/admin/model-providers',
	'/admin/model-access-policies',
	'/admin/license',
	'/admin/branding',
	'/admin/app-notification'
];

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

async function renderLayout(groups: string[] = [], versionOverrides: Partial<Version> = {}) {
	profile.initialize(createProfile(groups));
	version.initialize({
		...getVersionResponse,
		agentsEnabled: false,
		engine: 'docker',
		...versionOverrides
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

async function openAdvancedPane(label: 'Administration' | 'Advanced Settings') {
	const button = page.getByRole('button', { name: label, exact: true });
	await expect.element(button).toBeVisible();
	await clickButton('advanced-pane-btn');
}

async function expandSection(id: string, expectedHref: string) {
	const link = page.getByCSS(`a[href="${expectedHref}"]`);
	if ((await link.elements()).length === 0) {
		await clickButton(`sidebar-collapse-${id}`);
	}
}

async function expectLink(href: string) {
	await expect.element(page.getByCSS(`a[href="${href}"]`)).toBeInTheDocument();
}

async function expectAdminSections() {
	for (const label of adminSectionLabels) {
		await expect.element(page.getByText(label, { exact: true })).toBeVisible();
	}

	for (const { id, href } of adminSections) {
		await expandSection(id, href);
	}

	for (const href of adminSharedLinks) {
		await expectLink(href);
	}
}

describe('Layout.svelte', () => {
	it('gives all users access to non-administrative sidebar navigation', async () => {
		await renderLayout();

		for (const name of ['MCP Servers', 'Skills', 'Agent Auth Scopes']) {
			await expect.element(page.getByRole('link', { name, exact: true })).toBeVisible();
		}
	});

	describe('based on user role', () => {
		describe('when the user is an administrator', () => {
			it('shows administration sections and their navigation items', async () => {
				await renderLayout([Group.ADMIN]);
				await openAdvancedPane('Administration');
				await expectAdminSections();
				await expectLink('/admin/mcp-tunnels');
			});

			describe('when agents are enabled', () => {
				it('shows Obot Agent Management', async () => {
					await renderLayout([Group.ADMIN], { agentsEnabled: true });
					await openAdvancedPane('Administration');

					await expect
						.element(page.getByText('Obot Agent Management', { exact: true }))
						.toBeVisible();
				});
			});

			describe('when the Kubernetes engine is enabled', () => {
				it('shows server scheduling and image pull secrets navigation', async () => {
					await renderLayout([Group.ADMIN], { engine: 'kubernetes', hideK8sDetails: false });
					await openAdvancedPane('Administration');
					await expandSection('mcp-server-management', '/admin/mcp-catalog');

					await expectLink('/admin/server-scheduling');
					await expectLink('/admin/image-pull-secrets');
				});
			});
		});

		describe('when the user is a power user', () => {
			it('shows only the power-user MCP Management navigation', async () => {
				await renderLayout([Group.POWERUSER]);
				await openAdvancedPane('Advanced Settings');
				await expect.element(page.getByText('MCP Management', { exact: true })).toBeVisible();
				await expandSection('mcp-server-management', '/mcp-catalog');

				for (const href of ['/mcp-catalog', '/audit-logs', '/usage']) {
					await expectLink(href);
				}
				await expect
					.element(page.getByCSS('a[href="/mcp-access-policies"]'))
					.not.toBeInTheDocument();
			});
		});

		describe('when the user is a power user plus', () => {
			it('includes MCP access policies in MCP Management navigation', async () => {
				await renderLayout([Group.POWERUSER, Group.POWERUSER_PLUS]);
				await openAdvancedPane('Advanced Settings');
				await expect.element(page.getByText('MCP Management', { exact: true })).toBeVisible();
				await expandSection('mcp-server-management', '/mcp-catalog');

				for (const href of ['/mcp-catalog', '/mcp-access-policies', '/audit-logs', '/usage']) {
					await expectLink(href);
				}
			});
		});

		describe('when the user is a basic user', () => {
			it('does not show administration or advanced settings', async () => {
				await renderLayout([Group.USER]);

				await expect
					.element(page.getByRole('button', { name: 'Administration', exact: true }))
					.not.toBeInTheDocument();
				await expect
					.element(page.getByRole('button', { name: 'Advanced Settings', exact: true }))
					.not.toBeInTheDocument();
			});
		});

		describe('when the user is a basic user and auditor', () => {
			it('shows the administrator navigation available to auditors', async () => {
				await renderLayout([Group.USER, Group.AUDITOR]);
				await openAdvancedPane('Administration');
				await expectAdminSections();
			});
		});
	});
});
