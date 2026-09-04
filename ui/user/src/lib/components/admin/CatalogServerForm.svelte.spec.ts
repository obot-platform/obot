import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { MCPCatalogEntry } from '$lib/services';
import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { createMCPCatalogEntryResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const catalogID = 'test-catalog';

beforeEach(() => {
	worker.use(
		http.get('/api/mcp-catalogs/default/access-control-rules', () => HttpResponse.json([]))
	);
});

async function renderHostedForm(onSubmit = vi.fn()) {
	await render(CatalogServerForm, {
		id: catalogID,
		entity: 'catalog',
		type: 'hosted',
		onSubmit
	});

	return onSubmit;
}

async function renderRemoteForm(onSubmit = vi.fn()) {
	await render(CatalogServerForm, {
		id: catalogID,
		entity: 'catalog',
		type: 'remote',
		onSubmit
	});

	return onSubmit;
}

async function fillRequiredServerFields({
	name = createMCPCatalogEntryResponse.manifest.name,
	shortDescription = createMCPCatalogEntryResponse.manifest.shortDescription,
	packageName = createMCPCatalogEntryResponse.manifest.npxConfig.package
} = {}) {
	if (name) {
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill(name);
	}
	if (shortDescription) {
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill(shortDescription);
	}
	if (packageName) {
		await page.getByCSS('#npx-package').fill(packageName);
	}
}

async function submitForm() {
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
}

function mockCatalogEntrySubmit() {
	worker.use(
		http.post(`/api/mcp-catalogs/${catalogID}/entries`, () => {
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
}

async function addConfiguration() {
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();
}

async function selectStaticConfiguration(index = 0) {
	await page.getByCSS(`#env-value-type-${CATALOG_SERVER_FIELD_IDS.env}-${index}`).click();
	await page.getByRole('button', { name: 'Static', exact: true }).click();
}

describe('CatalogServerForm.svelte', () => {
	it('shows a required validation indicator when Name is empty', async () => {
		await renderHostedForm();
		await fillRequiredServerFields({ name: '' });

		await submitForm();

		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
			.toHaveAttribute('aria-invalid', 'true');
		await expect.element(page.getByText('Name is required', { exact: true })).toBeVisible();
	});

	it('shows a required validation indicator when Short Description is empty', async () => {
		await renderHostedForm();
		await fillRequiredServerFields({ shortDescription: '' });

		await submitForm();

		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
			.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByText('Short description is required', { exact: true }))
			.toBeVisible();
	});

	describe('hosted catalog entry', () => {
		it('shows required validation indicators for an empty Configuration Key and Name', async () => {
			await renderHostedForm();
			await fillRequiredServerFields();
			await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();

			await submitForm();

			await expect
				.element(page.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
			await expect
				.element(page.getByCSS(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
		});

		it('submits a valid hosted catalog entry', async () => {
			worker.use(
				http.post(`/api/mcp-catalogs/${catalogID}/entries`, async () => {
					return HttpResponse.json(createMCPCatalogEntryResponse);
				})
			);
			const onSubmit = await renderHostedForm();
			await fillRequiredServerFields();
			await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();
			await page.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('TEST_API_KEY');
			await page.getByCSS(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('Test API Key');

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});
		it('shows required validation indicators for an empty static configuration', async () => {
			await renderHostedForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await selectStaticConfiguration();

			await submitForm();

			await expect
				.element(page.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
			await expect
				.element(page.getByCSS(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
		});

		it('submits a valid entry with user-supplied configuration', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderHostedForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await page
				.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.config[0].key);
			await page
				.getByCSS(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.config[0].name);

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});

		it('submits a valid entry with static configuration', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderHostedForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await selectStaticConfiguration();
			await page
				.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.config[0].key);
			await page
				.getByCSS(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill('test-api-key-value');

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});

		it('edits and submits every configuration usage without legacy fields', async () => {
			const entry: MCPCatalogEntry = structuredClone(createMCPCatalogEntryResponse);
			const usages = ['env', 'header', 'file', 'dynamicFile', 'interpolated'] as const;
			entry.manifest.config = usages.map((usage, index) => ({
				key: `CONFIG_${index}`,
				name: '',
				description: '',
				required: false,
				sensitive: false,
				value: 'value',
				userAllowed: true,
				usage
			}));
			let submitted: Record<string, unknown> | undefined;
			worker.use(
				http.put(`/api/mcp-catalogs/${catalogID}/entries/${entry.id}`, async ({ request }) => {
					submitted = (await request.json()) as Record<string, unknown>;
					return HttpResponse.json(entry);
				})
			);

			await render(CatalogServerForm, { id: catalogID, entity: 'catalog', entry });
			await page.getByCSS('#catalog-config-usage-0').click();
			for (const usage of [
				'Environment Variable',
				'Header',
				'File',
				'Dynamic File',
				'Interpolated Value'
			]) {
				await expect.element(page.getByRole('button', { name: usage, exact: true })).toBeVisible();
			}
			await page.getByRole('button', { name: 'Header', exact: true }).click();
			await submitForm();

			await vi.waitFor(() => expect(submitted).toBeDefined());
			expect(submitted).not.toHaveProperty('env');
			expect(submitted).not.toHaveProperty('serverUserType');
			expect(submitted).not.toHaveProperty('multiUserConfig');
			expect(submitted).not.toHaveProperty('remoteConfig');
			for (const field of submitted?.config as Record<string, unknown>[]) {
				expect(field).not.toHaveProperty('userAllowed');
			}
			expect((submitted?.config as { usage: string }[]).map(({ usage }) => usage)).toEqual([
				'header',
				'header',
				'file',
				'dynamicFile',
				'interpolated'
			]);
		});

		it('submits flattened deployed server configuration and stores values separately', async () => {
			const server = createMCPCatalogServer({
				id: 'legacy-server',
				name: 'Legacy server',
				userID: 'user-1',
				serverUserType: 'multiUser',
				env: [
					{
						key: 'LEGACY_KEY',
						name: 'Legacy key',
						description: '',
						required: false,
						sensitive: false,
						value: ''
					}
				]
			});
			let submitted: Record<string, unknown> | undefined;
			let configured: Record<string, unknown> | undefined;
			worker.use(
				http.put(`/api/mcp-catalogs/${catalogID}/servers/${server.id}`, async ({ request }) => {
					submitted = (await request.json()) as Record<string, unknown>;
					return HttpResponse.json(server);
				}),
				http.post(
					`/api/mcp-catalogs/${catalogID}/servers/${server.id}/reveal`,
					() => new HttpResponse(null, { status: 404 })
				),
				http.post(
					`/api/mcp-catalogs/${catalogID}/servers/${server.id}/configure`,
					async ({ request }) => {
						configured = (await request.json()) as Record<string, unknown>;
						return HttpResponse.json(server);
					}
				)
			);

			await render(CatalogServerForm, { id: catalogID, entity: 'catalog', entry: server });
			await selectStaticConfiguration();
			await page.getByCSS(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('legacy-value');
			await submitForm();

			await vi.waitFor(() => expect(submitted).toBeDefined());
			await vi.waitFor(() => expect(configured).toEqual({ LEGACY_KEY: 'legacy-value' }));
			expect(submitted?.config).toEqual(
				expect.arrayContaining([
					expect.objectContaining({ key: 'LEGACY_KEY', usage: 'env', value: '' })
				])
			);
			expect(submitted).not.toHaveProperty('env');
		});

		it('validates duplicate configuration keys for workspace catalog entries', async () => {
			const createEntry = vi.fn();
			worker.use(
				http.post(`/api/workspaces/${catalogID}/entries`, async () => {
					createEntry();
					return HttpResponse.json(createMCPCatalogEntryResponse);
				})
			);
			await render(CatalogServerForm, { id: catalogID, entity: 'workspace', type: 'hosted' });
			await fillRequiredServerFields();
			await addConfiguration();
			await addConfiguration();
			await selectStaticConfiguration(0);
			await selectStaticConfiguration(1);
			for (const index of [0, 1]) {
				await page.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-${index}`).fill('DUPLICATE');
				await page.getByCSS(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-${index}`).fill('value');
			}

			await submitForm();

			expect(createEntry).not.toHaveBeenCalled();
		});
	});

	describe('remote catalog entry', () => {
		it('shows a required validation indicator when URL is empty', async () => {
			await renderRemoteForm();
			await fillRequiredServerFields({ packageName: '' });

			await submitForm();

			await expect.element(page.getByCSS('#basic-url')).toHaveClass(/error/);
		});

		it('submits a valid remote catalog entry', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderRemoteForm();
			await fillRequiredServerFields({ packageName: '' });
			await page.getByCSS('#basic-url').fill('https://example.com/mcp');

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});
	});
});
