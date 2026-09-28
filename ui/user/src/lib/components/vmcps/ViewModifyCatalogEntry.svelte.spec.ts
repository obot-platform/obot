import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { Group } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ViewModifyCatalogEntry from './ViewModifyCatalogEntry.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('catalog entry hydration', () => {
	it.each([
		{ role: 'user', groups: [], endpoint: '/api/all-mcps/entries' },
		{ role: 'admin', groups: [Group.ADMIN], endpoint: '/api/mcp-catalogs/default/entries' },
		{ role: 'auditor', groups: [Group.AUDITOR], endpoint: '/api/mcp-catalogs/default/entries' }
	])('loads details through the $role endpoint', async ({ groups, endpoint }) => {
		const entry = createMCPCatalogEntry({ id: 'entry-hydration', name: 'Initial entry' });
		const requested = vi.fn();
		worker.use(
			http.get(`${endpoint}/${entry.id}`, () => {
				requested();
				return HttpResponse.json({
					...entry,
					manifest: { ...entry.manifest, name: 'Hydrated entry' }
				});
			})
		);
		await preparePageData({ profile: createMockProfile(groups) });
		const { component } = await render(ViewModifyCatalogEntry);
		await component.open(entry);
		expect(requested).toHaveBeenCalledOnce();
		await expect
			.element(page.getByRole('heading', { name: 'Hydrated entry', exact: true }).first())
			.toBeVisible();
	});
});

describe('OpenAPI creation', () => {
	it.each([
		{ groups: [Group.ADMIN], collection: 'mcp-catalogs', id: 'default' },
		{ groups: [Group.POWERUSER], collection: 'workspaces', id: 'workspace-test' }
	])('imports before opening configuration in $collection', async ({ groups, collection, id }) => {
		const imported = vi.fn();
		worker.use(
			http.post(`/api/${collection}/${id}/openapi/import`, async ({ request }) => {
				imported(await request.json());
				return HttpResponse.json({
					schema: {
						openapi: '3.1.0',
						info: { title: 'Example', version: '1' },
						servers: [{ url: 'https://example.com/api' }],
						paths: {}
					},
					baseURL: 'https://example.com/api',
					suggestedMetadata: {
						name: 'Example API',
						description: 'Imported description',
						shortDescription: 'API summary'
					},
					suggestedHeaders: [
						{
							key: 'Authorization',
							name: 'API key',
							usage: 'header',
							prefix: 'Bearer ',
							required: true,
							sensitive: true
						}
					]
				});
			})
		);
		await preparePageData({ profile: createMockProfile(groups) });
		const { component } = await render(ViewModifyCatalogEntry, { workspaceId: 'workspace-test' });
		component.start();
		await page.getByRole('button', { name: /OpenAPI/ }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Import OpenAPI schema' }))
			.toBeVisible();
		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
			.not.toBeInTheDocument();
		await expect.element(page.getByLabelText('API request base URL')).not.toBeInTheDocument();
		await page
			.getByLabelText('Schema URL', { exact: true })
			.fill('https://example.com/openapi.json');
		await page.getByRole('button', { name: 'Import schema', exact: true }).click();
		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
			.toHaveValue('Example API');
		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
			.toHaveValue('API summary');
		await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Bearer ');
		await expect.element(page.getByLabelText('API request base URL')).toHaveValue('');
		await expect
			.element(page.getByLabelText('API request base URL'))
			.toHaveAttribute('placeholder', 'https://example.com/api');
		await expect.element(page.getByCSS('#runtime-selector')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Replace schema' })).toBeVisible();
		expect(imported).toHaveBeenCalledOnce();
	});

	it('returns to the type picker when import is cancelled', async () => {
		await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
		const { component } = await render(ViewModifyCatalogEntry);
		component.start();
		await page.getByRole('button', { name: /OpenAPI/ }).click();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await expect.element(page.getByRole('heading', { name: 'Select Server Type' })).toBeVisible();
		await page.getByRole('button', { name: /OpenAPI/ }).click();
		await expect.element(page.getByLabelText('Schema URL', { exact: true })).toHaveValue('');
	});
});
