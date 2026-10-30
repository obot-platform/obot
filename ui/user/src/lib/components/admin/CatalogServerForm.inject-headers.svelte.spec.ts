import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { MCPCatalogEntryServerManifest } from '$lib/services';
import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { createMCPCatalogEntryResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('catalog attribution headers', () => {
	it('clears saved headers when the master toggle is disabled', async () => {
		const entry = createMCPCatalogEntry({
			id: 'jira',
			name: 'Jira',
			runtime: 'remote',
			manifest: {
				remoteConfig: { fixedURL: 'https://example.com/mcp' },
				injectObotHeaders: ['X-Obot-User-Id']
			}
		});
		let submitted: MCPCatalogEntryServerManifest | undefined;
		worker.use(
			http.put('/api/mcp-catalogs/test/entries/jira', async ({ request }) => {
				submitted = (await request.json()) as MCPCatalogEntryServerManifest;
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'remote',
			entry,
			onSubmit: vi.fn()
		});
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }))
			.toBeChecked();
		await page.getByRole('switch', { name: 'Disable Obot header injection', exact: true }).click();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }))
			.not.toBeInTheDocument();
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
		await vi.waitFor(() => expect(submitted?.injectObotHeaders).toEqual([]));
	});

	it('loads saved selections and saves header changes', async () => {
		const entry = createMCPCatalogEntry({
			id: 'jira',
			name: 'Jira',
			runtime: 'remote',
			manifest: {
				remoteConfig: { fixedURL: 'https://example.com/mcp' },
				injectObotHeaders: ['X-Obot-User-Id']
			}
		});
		let submitted: MCPCatalogEntryServerManifest | undefined;
		worker.use(
			http.put('/api/mcp-catalogs/test/entries/jira', async ({ request }) => {
				submitted = (await request.json()) as MCPCatalogEntryServerManifest;
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'remote',
			entry,
			onSubmit: vi.fn()
		});
		const userID = page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true });
		await expect.element(userID).toBeChecked();
		await userID.click();
		await expect.element(userID).not.toBeChecked();
		await userID.click();
		await page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }).click();
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
		await vi.waitFor(() =>
			expect(submitted?.injectObotHeaders).toEqual(['X-Obot-Api-Key-Name', 'X-Obot-User-Id'])
		);
	});

	it('saves selected headers on a remote catalog entry', async () => {
		let submitted: MCPCatalogEntryServerManifest | undefined;
		worker.use(
			http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
				submitted = (await request.json()) as MCPCatalogEntryServerManifest;
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'remote',
			onSubmit: vi.fn()
		});
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill('Jira');
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('Jira tools');
		await page.getByCSS('#basic-url').fill('https://example.com/mcp');
		await expect
			.element(page.getByText('Inject Obot headers', { exact: true }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Advanced Configuration', exact: true }).click();
		await page.getByRole('switch', { name: 'Enable Obot header injection', exact: true }).click();
		await page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }).click();
		await page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }).click();
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
		await vi.waitFor(() =>
			expect(submitted?.injectObotHeaders).toEqual(['X-Obot-Api-Key-Name', 'X-Obot-User-Id'])
		);
	});

	it('reloads saved selections', async () => {
		const entry = createMCPCatalogEntry({
			id: 'jira',
			name: 'Jira',
			runtime: 'remote',
			manifest: {
				remoteConfig: { fixedURL: 'https://example.com/mcp' },
				injectObotHeaders: ['X-Obot-User-Email']
			}
		});
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'remote',
			entry,
			onSubmit: vi.fn()
		});
		await expect
			.element(page.getByRole('button', { name: 'Reset Default Configuration', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Email', exact: true }))
			.toBeChecked();
	});

	it('does not offer HTTP headers for STDIO servers', async () => {
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'hosted',
			onSubmit: vi.fn()
		});
		await expect
			.element(page.getByText('Inject Obot headers', { exact: true }))
			.not.toBeInTheDocument();
	});

	it.each(['remote', 'containerized'] as const)(
		'saves remote-only attribution settings on a deployed %s server',
		async (runtime) => {
			const server = createMCPCatalogServer({
				id: 'jira-server',
				name: 'Jira',
				userID: 'user-1',
				runtime,
				manifest: {
					containerizedConfig:
						runtime === 'containerized'
							? { image: 'example/jira:latest', port: 8080, path: '/mcp' }
							: undefined,
					injectObotHeaders: ['X-Obot-User-Id']
				}
			});
			let submitted: MCPCatalogEntryServerManifest | undefined;
			worker.use(
				http.post('/api/mcp-catalogs/test/servers/jira-server/reveal', () => HttpResponse.json({})),
				http.put('/api/mcp-catalogs/test/servers/jira-server', async ({ request }) => {
					submitted = (await request.json()) as MCPCatalogEntryServerManifest;
					return HttpResponse.json(server);
				})
			);
			await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry: server });
			if (runtime === 'containerized') {
				await expect
					.element(page.getByText('Inject Obot headers', { exact: true }))
					.not.toBeInTheDocument();
				await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
				await vi.waitFor(() => {
					expect(submitted).toBeDefined();
					expect(submitted?.injectObotHeaders).toBeUndefined();
				});
				return;
			}
			await expect
				.element(page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }))
				.toBeChecked();
			await page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }).click();
			await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
			await vi.waitFor(() =>
				expect(submitted?.injectObotHeaders).toEqual(['X-Obot-Api-Key-Name', 'X-Obot-User-Id'])
			);
		}
	);
});
