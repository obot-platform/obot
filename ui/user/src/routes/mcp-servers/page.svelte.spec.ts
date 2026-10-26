import { page as appPage } from '$app/state';
import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { Group } from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { createDeploymentsPageFixtures } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import McpServersPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$app/navigation', async () => {
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		goto: (url: string | URL) => applyTestGoto(url),
		invalidate: vi.fn(),
		invalidateAll: vi.fn(),
		preloadCode: vi.fn(),
		preloadData: vi.fn(),
		beforeNavigate: vi.fn(),
		afterNavigate: vi.fn(),
		onNavigate: vi.fn(),
		pushState: vi.fn(),
		replaceState: vi.fn(),
		disableScrollHandling: vi.fn()
	};
});

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url)
	};
});

const fixtures = createDeploymentsPageFixtures();

// The page loads the default catalog in onMount only after other requests resolve. Tests wait for it so that the
// request doesn't escape the mocks after the test ends.
let defaultCatalogRequests = 0;

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
		http.get('/api/mcp-catalogs/default', () => {
			defaultCatalogRequests++;
			return HttpResponse.json({
				id: 'default',
				displayName: 'Default',
				sourceURLs: [],
				allowedUserIDs: []
			});
		}),
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
		http.get('/api/all-mcps/servers', () => HttpResponse.json({ items: fixtures.servers })),
		http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [] }))
	);
}

async function renderMcpServersPage({
	view = 'deployments',
	groups = [Group.ADMIN]
}: {
	view?: string;
	groups?: string[];
} = {}) {
	(appPage.url as URL).searchParams.set('view', view);
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	mockDeploymentsApis();
	const data = await preparePageData<PageData>({
		profile: createMockProfile(groups),
		workspaceId: undefined,
		gitCredentials: [],
		filters: [],
		systemCatalogEntries: [],
		mcpTunnels: [],
		tunnelConnections: undefined,
		accessControlRules: []
	});
	defaultCatalogRequests = 0;
	const result = render(McpServersPage, { data });
	if (data.profile.hasAdminAccess?.())
		await expect.poll(() => defaultCatalogRequests).toBeGreaterThan(0);
	return result;
}

async function waitForServersLoaded() {
	await expect
		.element(
			page.getByRole('row').filter({ hasText: fixtures.serverSingle.manifest.name! }).first()
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

	for (const label of ['View Audit Logs', ...(expected.absent ?? [])]) {
		await expect
			.element(page.getByRole('button', { name: label, exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: label, exact: true }))
			.not.toBeInTheDocument();
	}
}

afterEach(() => {
	(appPage.url as URL).searchParams.delete('view');
	(appPage.url as URL).searchParams.delete('new');
});

it('imports OpenAPI from the type picker before opening configuration', async () => {
	resetMcpServersAndEntriesStore();
	worker.use(
		http.post('/api/openapi-import', () =>
			HttpResponse.json({
				baseURL: '',
				destinationValidation: { valid: false, error: 'no usable server URL; configure baseURL' },
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Imported API' }
			})
		)
	);
	await renderMcpServersPage({ view: 'servers' });
	await page.getByRole('button', { name: 'Add MCP Server' }).click();
	await page.getByRole('button', { name: /OpenAPI/ }).click();
	await expect.element(page.getByRole('heading', { name: 'Import OpenAPI schema' })).toBeVisible();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).not.toBeInTheDocument();
	expect((appPage.url as URL).searchParams.get('new')).not.toBe('openapi');
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/schema.json');
	await page.getByRole('button', { name: 'Import', exact: true }).click();
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
		.toHaveValue('Imported API');
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toBeRequired();
	await expect.element(page.getByCSS('#runtime-selector')).not.toBeInTheDocument();
	expect((appPage.url as URL).searchParams.get('new')).toBe('openapi');
});

describe('MCP Servers Page', () => {
	describe('filters tab', () => {
		beforeEach(() => {
			resetMcpServersAndEntriesStore();
			worker.use(
				http.get('/api/llm-gateway/guardrails', () =>
					HttpResponse.json({ items: [{ id: 'gp1secrets', name: 'Secrets', type: 'secrets' }] })
				),
				http.get('/api/llm-gateway/guardrails/providers', () => HttpResponse.json({ items: [] })),
				http.get('/api/llm-gateway/guardrail-policies', () => HttpResponse.json({ items: [] })),
				http.get('/api/mcp-webhook-validations', () =>
					HttpResponse.json({
						items: [
							{
								id: 'f1',
								name: 'No secrets',
								guardrail: { guardrailIDs: ['gp1secrets'], action: 'redact' }
							}
						]
					})
				)
			);
		});

		afterEach(() => {
			(appPage.url as URL).searchParams.delete('new');
		});

		it('offers Create From Guardrail in the Add New Filter menu', async () => {
			await renderMcpServersPage({ view: 'filters' });

			await page.getByRole('button', { name: 'Add New Filter' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Create Custom', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Create From Built-in', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Create From Guardrail', exact: true }))
				.toBeEnabled();
		});

		it('has no Guardrails tab; guardrails are managed under LLM Gateway', async () => {
			await renderMcpServersPage({ view: 'filters' });

			await expect
				.element(page.getByRole('button', { name: 'Filters', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Guardrails', exact: true }))
				.not.toBeInTheDocument();
		});

		it('opens the guardrail filter form for new=guardrail', async () => {
			(appPage.url as URL).searchParams.set('new', 'guardrail');
			await renderMcpServersPage({ view: 'filters' });

			// The layout title repeats the heading.
			await expect
				.element(page.getByRole('heading', { name: 'Create Guardrail Filter' }).last())
				.toBeVisible();
			await expect.element(page.getByRole('checkbox', { name: /Secrets/ })).toBeVisible();
			await expect.element(page.getByLabelText('Webhook URL')).not.toBeInTheDocument();
		});

		it('opens the custom filter form for other new values', async () => {
			(appPage.url as URL).searchParams.set('new', 'true');
			await renderMcpServersPage({ view: 'filters' });

			await expect
				.element(page.getByRole('heading', { name: 'Create Filter', exact: true }).last())
				.toBeVisible();
			await expect.element(page.getByLabelText('Webhook URL')).toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Create Guardrail Filter' }))
				.not.toBeInTheDocument();
		});
	});

	describe('deployments tab', () => {
		beforeEach(async () => {
			resetMcpServersAndEntriesStore();
			await renderMcpServersPage();
			await waitForServersLoaded();
		});

		describe('ellipsis menu actions', () => {
			it('single-user npx shows edit, restart, delete', async () => {
				await expectMenuActions(fixtures.serverSingle.manifest.name!, {
					links: ['View Catalog Entry'],
					present: ['Edit Configuration', 'Restart Server', 'Delete Server'],
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
						'Delete Server'
					]
				});
			});

			it('multi-user npx shows catalog, edit, restart, delete', async () => {
				await expectMenuActions(fixtures.serverMulti.manifest.name!, {
					links: ['View Catalog Entry'],
					present: ['Edit Configuration', 'Restart Server', 'Delete Server'],
					absent: ['Update Scheduling Config']
				});
			});

			it('remote server omits edit configuration and restart', async () => {
				await expectMenuActions(fixtures.serverRemote.manifest.name!, {
					links: ['View Catalog Entry'],
					present: ['Delete Server'],
					absent: ['Edit Configuration', 'Restart Server', 'Update Scheduling Config']
				});
			});

			it('server without catalogEntryID shows View Server instead of View Catalog Entry', async () => {
				await expectMenuActions(fixtures.serverNoCatalogEntry.manifest.name!, {
					links: ['View Server'],
					present: ['Edit Configuration', 'Restart Server', 'Delete Server'],
					absent: ['View Catalog Entry', 'Update Scheduling Config']
				});
			});
		});

		describe('multi-select action pills', () => {
			it('shows restart, k8s upgrade, and delete counts for selected rows', async () => {
				await page.getByRole('columnheader').first().getByRole('button').click({ force: true });

				await expect.element(page.getByText(/of 5 selected/)).toBeVisible();

				const actionsBar = page.getByText(/of 5 selected/).locator('..');

				await expect
					.element(
						actionsBar.getByRole('button', { name: /^Restart/ }).getByText('4', { exact: true })
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
						actionsBar.getByRole('button', { name: /^Delete/ }).getByText('5', { exact: true })
					)
					.toBeVisible();
			});
		});
	});
});

it.each([
	{ role: 'power user', groups: [Group.POWERUSER] },
	{ role: 'power user plus', groups: [Group.POWERUSER_PLUS, Group.POWERUSER] },
	{ role: 'auditor', groups: [Group.AUDITOR] },
	{ role: 'basic user', groups: [] }
])('does not open the direct OpenAPI creation URL for $role', async ({ groups }) => {
	resetMcpServersAndEntriesStore();
	(appPage.url as URL).searchParams.set('new', 'openapi');
	await renderMcpServersPage({ view: 'entries', groups });
	await expect
		.element(page.getByRole('heading', { name: 'Import OpenAPI schema' }))
		.not.toBeInTheDocument();
	await expect
		.element(page.getByRole('button', { name: 'Import', exact: true }))
		.not.toBeInTheDocument();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).not.toBeInTheDocument();
});

it('allows an owner to import OpenAPI from the type picker', async () => {
	resetMcpServersAndEntriesStore();
	await renderMcpServersPage({ view: 'servers', groups: [Group.OWNER, Group.ADMIN] });
	await page.getByRole('button', { name: 'Add MCP Server' }).click();
	await page.getByRole('button', { name: /OpenAPI/ }).click();
	await expect.element(page.getByRole('heading', { name: 'Import OpenAPI schema' })).toBeVisible();
	expect((appPage.url as URL).searchParams.get('new')).not.toBe('openapi');
});
