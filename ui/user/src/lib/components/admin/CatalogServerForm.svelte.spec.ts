import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { createMCPCatalogEntryResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/node';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const catalogID = 'test-catalog';

async function renderSingleTenantForm(onSubmit = vi.fn()) {
	await render(CatalogServerForm, {
		id: catalogID,
		entity: 'catalog',
		type: 'hosted',
		onSubmit
	});

	await page.locator('#server-configuration-selector').click();
	await page.getByRole('button', { name: 'Single-tenant', exact: true }).click();

	return onSubmit;
}

async function renderMultiTenantForm(onSubmit = vi.fn()) {
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
		await page.locator(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill(name);
	}
	if (shortDescription) {
		await page.locator(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill(shortDescription);
	}
	if (packageName) {
		await page.locator('#npx-package').fill(packageName);
	}
}

async function submitForm() {
	await page.locator(`#${CATALOG_SERVER_FIELD_IDS.submitBtn}`).click();
}

function mockCatalogEntrySubmit() {
	worker.use(
		http.post(`/api/mcp-catalogs/${catalogID}/entries`, () => {
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
}

async function addConfiguration() {
	await page.locator(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();
}

async function selectStaticConfiguration() {
	await page.locator(`#env-value-type-${CATALOG_SERVER_FIELD_IDS.env}-0`).click();
	const staticOption = Array.from(document.querySelectorAll('button')).find(
		(button) => button.textContent?.trim() === 'Static'
	);
	if (!staticOption) {
		throw new Error('Expected the Static configuration option to be rendered');
	}
	staticOption.click();
	await tick();
}

describe('CatalogServerForm.svelte', () => {
	it('shows a required validation indicator when Name is empty', async () => {
		await renderSingleTenantForm();
		await fillRequiredServerFields({ name: '' });

		await submitForm();

		await expect
			.element(page.locator(`#${CATALOG_SERVER_FIELD_IDS.name}`))
			.toHaveAttribute('aria-invalid', 'true');
		await expect.element(page.getByText('Name is required', { exact: true })).toBeVisible();
	});

	it('shows a required validation indicator when Short Description is empty', async () => {
		await renderSingleTenantForm();
		await fillRequiredServerFields({ shortDescription: '' });

		await submitForm();

		await expect
			.element(page.locator(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
			.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByText('Short description is required', { exact: true }))
			.toBeVisible();
	});

	describe('single-tenant hosted catalog entry', () => {
		it('shows required validation indicators for an empty Configuration Key and Name', async () => {
			await renderSingleTenantForm();
			await fillRequiredServerFields();
			await page.locator(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();

			await submitForm();

			await expect
				.element(page.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
			await expect
				.element(page.locator(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
		});

		it('submits a valid single-tenant hosted catalog entry', async () => {
			worker.use(
				http.post(`/api/mcp-catalogs/${catalogID}/entries`, async () => {
					return HttpResponse.json(createMCPCatalogEntryResponse);
				})
			);
			const onSubmit = await renderSingleTenantForm();
			await fillRequiredServerFields();
			await page.locator(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();
			await page.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('TEST_API_KEY');
			await page.locator(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('Test API Key');

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});
	});

	describe('multi-tenant hosted catalog entry', () => {
		it('shows required validation indicators for an empty User-Supplied Key and Name', async () => {
			await renderMultiTenantForm();
			await fillRequiredServerFields();
			await addConfiguration();

			await submitForm();

			await expect
				.element(page.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
			await expect
				.element(page.locator(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
		});

		it('shows required validation indicators for an empty Static Key and Value', async () => {
			await renderMultiTenantForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await selectStaticConfiguration();

			await submitForm();

			await expect
				.element(page.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
			await expect
				.element(page.locator(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-0`))
				.toHaveClass(/error/);
		});

		it('submits a valid entry with User-Supplied Configuration', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderMultiTenantForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await page
				.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.env[0].key);
			await page
				.locator(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.env[0].name);

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});

		it('submits a valid entry with Static Configuration', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderMultiTenantForm();
			await fillRequiredServerFields();
			await addConfiguration();
			await selectStaticConfiguration();
			await page
				.locator(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-0`)
				.fill(createMCPCatalogEntryResponse.manifest.env[0].key);
			await page.locator(`#env-value-${CATALOG_SERVER_FIELD_IDS.env}-0`).fill('test-api-key-value');

			await submitForm();

			await vi.waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					createMCPCatalogEntryResponse,
					'Catalog entry updated successfully!'
				);
			});
		});
	});

	describe('remote catalog entry', () => {
		it('shows a required validation indicator when URL is empty', async () => {
			await renderRemoteForm();
			await fillRequiredServerFields({ packageName: '' });

			await submitForm();

			await expect.element(page.locator('#basic-url')).toHaveClass(/error/);
		});

		it('submits a valid remote catalog entry', async () => {
			mockCatalogEntrySubmit();
			const onSubmit = await renderRemoteForm();
			await fillRequiredServerFields({ packageName: '' });
			await page.locator('#basic-url').fill('https://example.com/mcp');

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
