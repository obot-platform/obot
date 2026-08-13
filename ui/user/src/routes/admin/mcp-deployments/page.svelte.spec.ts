import { mcpServersAndEntries } from '$lib/stores';
import { preparePageData } from '../../../tests/helpers/pageData';
import { createDeploymentsPageFixtures } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import DeploymentsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createDeploymentsPageFixtures();

function resetMcpServersAndEntriesStore() {
	mcpServersAndEntries.current = {
		entries: [],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: false
	};
}

function mockDeploymentsApis() {
	worker.use(
		http.get('/api/mcp-catalogs/default/servers', () =>
			HttpResponse.json({ items: fixtures.servers })
		),
		http.get('/api/mcp-catalogs/default/entries', () =>
			HttpResponse.json({ items: fixtures.entries })
		),
		http.get('/api/mcp-catalogs/default/entries/all-servers', () =>
			HttpResponse.json({ items: [] })
		),
		http.get('/api/workspaces/all-entries/all-servers', () => HttpResponse.json({ items: [] })),
		http.get('/api/workspaces/all-entries', () => HttpResponse.json({ items: [] })),
		http.get('/api/workspaces/all-servers', () => HttpResponse.json({ items: [] })),
		http.get('/api/all-mcps/entries', () => HttpResponse.json({ items: fixtures.entries })),
		http.get('/api/all-mcps/servers', () => HttpResponse.json({ items: fixtures.servers }))
	);
}

async function renderDeploymentsPage() {
	mockDeploymentsApis();
	await preparePageData();
	return render(DeploymentsPage);
}

async function waitForServersLoaded() {
	await expect
		.element(
			page
				.getByRole('row')
				.filter({ hasText: fixtures.serverSingleNeedsUpdate.manifest.name! })
				.first()
		)
		.toBeVisible();
}

async function openRowActions(serverName: string) {
	const row = page
		.getByRole('row')
		.filter({ hasText: serverName })
		.filter({ hasNotText: `(${serverName})` });
	await row.getByRole('button', { name: 'Row actions' }).click();
}

async function expectMenuActions(
	serverName: string,
	expected: {
		present: string[];
		absent?: string[];
		links?: string[];
	}
) {
	await openRowActions(serverName);

	for (const label of expected.links ?? []) {
		await expect.element(page.getByRole('link', { name: label, exact: true })).toBeVisible();
	}

	for (const label of expected.present) {
		await expect.element(page.getByRole('button', { name: label, exact: true })).toBeVisible();
	}

	for (const label of expected.absent ?? []) {
		await expect
			.element(page.getByRole('button', { name: label, exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: label, exact: true }))
			.not.toBeInTheDocument();
	}
}

describe('MCP Deployments Page', () => {
	beforeEach(async () => {
		resetMcpServersAndEntriesStore();
		await renderDeploymentsPage();
		await waitForServersLoaded();
	});

	describe('ellipsis menu actions', () => {
		it('single-user npx with needsUpdate shows update, diff, edit, restart, audit, delete', async () => {
			await expectMenuActions(fixtures.serverSingleNeedsUpdate.manifest.name!, {
				links: ['View Catalog Entry'],
				present: [
					'Edit Configuration',
					'Update Server',
					'View Diff',
					'Restart Server',
					'View Audit Logs',
					'Delete Server'
				],
				absent: ['Update Scheduling Config']
			});
		});

		it('single-user npx with needsK8sUpdate shows scheduling config update', async () => {
			await expectMenuActions(fixtures.serverSingleNeedsK8s.manifest.name!, {
				links: ['View Catalog Entry'],
				present: [
					'Edit Configuration',
					'Update Scheduling Config',
					'Restart Server',
					'View Audit Logs',
					'Delete Server'
				],
				absent: ['Update Server', 'View Diff']
			});
		});

		it('multi-user npx shows catalog, edit, restart, audit, delete', async () => {
			await expectMenuActions(fixtures.serverMulti.manifest.name!, {
				links: ['View Catalog Entry'],
				present: ['Edit Configuration', 'Restart Server', 'View Audit Logs', 'Delete Server'],
				absent: ['Update Server', 'View Diff', 'Update Scheduling Config']
			});
		});

		it('remote server omits edit configuration and restart', async () => {
			await expectMenuActions(fixtures.serverRemote.manifest.name!, {
				links: ['View Catalog Entry'],
				present: ['View Audit Logs', 'Delete Server'],
				absent: [
					'Edit Configuration',
					'Restart Server',
					'Update Server',
					'View Diff',
					'Update Scheduling Config'
				]
			});
		});

		it('composite parent omits edit configuration and restart', async () => {
			await expectMenuActions(fixtures.serverComposite.manifest.name!, {
				links: ['View Catalog Entry'],
				present: ['View Audit Logs', 'Delete Server'],
				absent: [
					'Edit Configuration',
					'Restart Server',
					'Update Server',
					'View Diff',
					'Update Scheduling Config'
				]
			});
		});

		it('server without catalogEntryID shows View Server instead of View Catalog Entry', async () => {
			await expectMenuActions(fixtures.serverNoCatalogEntry.manifest.name!, {
				links: ['View Server'],
				present: ['Edit Configuration', 'Restart Server', 'View Audit Logs', 'Delete Server'],
				absent: ['View Catalog Entry', 'Update Server', 'View Diff', 'Update Scheduling Config']
			});
		});

		it('composite child shows parent audit logs and keeps restart', async () => {
			await openRowActions(fixtures.serverCompositeChild.manifest.name!);

			await expect
				.element(page.getByRole('link', { name: 'View Catalog Entry', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Restart Server', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Edit Configuration', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: /View Parent Server\s*Audit Logs/ }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Delete Server', exact: true }))
				.toBeVisible();

			await expect
				.element(page.getByRole('button', { name: 'Update Server', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'View Diff', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Update Scheduling Config', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'View Audit Logs', exact: true }))
				.not.toBeInTheDocument();
		});
	});

	describe('multi-select action pills', () => {
		it('shows restart, upgrade, k8s upgrade, and delete counts for selected rows', async () => {
			await page.getByRole('columnheader').first().getByRole('button').click();

			await expect.element(page.getByText(/of 7 selected/)).toBeVisible();

			const actionsBar = page.getByText(/of 7 selected/).locator('..');

			await expect
				.element(
					actionsBar.getByRole('button', { name: /^Restart/ }).getByText('5', { exact: true })
				)
				.toBeVisible();

			await expect
				.element(
					actionsBar.getByRole('button', { name: /^Upgrade/ }).getByText('1', { exact: true })
				)
				.toBeVisible();

			await expect
				.element(
					actionsBar
						.getByRole('button', { name: /^Kubernetes Upgrade/ })
						.getByText('1', { exact: true })
				)
				.toBeVisible();

			await expect
				.element(
					actionsBar.getByRole('button', { name: /^Delete/ }).getByText('6', { exact: true })
				)
				.toBeVisible();
		});
	});
});
