import { AdminService, type MCPCatalogEntry } from '$lib/services';
import type { OpenAPIDestinationValidation } from '$lib/services/openapi';
import * as openAPIService from '$lib/services/openapi';
import { errors } from '$lib/stores';
import { createMCPCatalogEntryResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/services/openapi', { spy: true });

const config = {
	source: { url: 'https://example.com/openapi.json' },
	baseURL: 'https://override.example.com'
};
const result = {
	baseURL: 'https://example.com/api',
	destinationValidation: { valid: true },
	suggestedHeaders: [],
	suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
};

async function renderForm(update = false, onCancel = vi.fn(), onSubmit = vi.fn()) {
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			runtime: 'openapi',
			config: [],
			openAPIConfig: config
		}
	} as MCPCatalogEntry;
	const view = await render(CatalogServerForm, {
		id: 'test',
		entity: 'catalog',
		type: 'openapi',
		entry: update ? entry : undefined,
		initialOpenAPIImport: update ? undefined : { config, result },
		onCancel,
		onSubmit
	});
	return {
		view,
		save: page.getByRole('button', { name: update ? 'Update' : 'Save', exact: true })
	};
}

afterEach(() => {
	vi.restoreAllMocks();
	vi.clearAllMocks();
	errors.items = [];
});

it.each(['HTTP failure', 'network failure'])(
	'reports a destination validation %s and allows retry',
	async (failure) => {
		let fail = true;
		const saved = vi.fn();
		worker.use(
			http.post('/api/openapi-import', () => {
				if (fail)
					return failure === 'HTTP failure'
						? new HttpResponse('Schema source is unavailable', { status: 400 })
						: HttpResponse.error();
				return HttpResponse.json(result);
			}),
			http.post('/api/mcp-catalogs/test/entries', async () => {
				saved();
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		const { save } = await renderForm();
		await save.click();
		await vi.waitFor(() => expect(errors.items).toHaveLength(1));
		if (failure === 'HTTP failure')
			expect(errors.items[0].message).toContain('Schema source is unavailable');
		await expect.element(save).toBeEnabled();
		expect(saved).not.toHaveBeenCalled();
		fail = false;
		await save.click();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		expect(errors.items).toHaveLength(1);
	}
);

it.each([
	{ exit: 'Cancel', update: false },
	{ exit: 'unmount', update: false },
	{ exit: 'Cancel', update: true },
	{ exit: 'unmount', update: true }
])(
	'prevents a pending $update update/create after $exit even if validation ignores abort',
	async ({ exit, update }) => {
		let finishValidation!: (value: OpenAPIDestinationValidation) => void;
		// Ignore abort deliberately to verify that a late successful result cannot mutate the catalog.
		const validation = vi.mocked(openAPIService.validateOpenAPIDestination).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishValidation = resolve;
				})
		);
		const saved = vi.fn();
		const mutation = vi.spyOn(
			AdminService,
			update ? 'updateMCPCatalogEntry' : 'createMCPCatalogEntry'
		);
		worker.use(
			http.post('/api/mcp-catalogs/test/entries', () => {
				saved();
				return HttpResponse.json(createMCPCatalogEntryResponse);
			}),
			http.put(`/api/mcp-catalogs/test/entries/${createMCPCatalogEntryResponse.id}`, () => {
				saved();
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		const onCancel = vi.fn();
		const onSubmit = vi.fn();
		const { view, save } = await renderForm(update, onCancel, onSubmit);
		await save.click();
		await vi.waitFor(() => expect(validation).toHaveBeenCalledOnce());
		const signal = validation.mock.calls[0][1];
		expect(signal?.aborted).toBe(false);
		if (exit === 'Cancel') {
			await page.getByRole('button', { name: 'Cancel', exact: true }).click();
			expect(onCancel).toHaveBeenCalledOnce();
		} else {
			await view.unmount();
		}
		expect(signal?.aborted).toBe(true);
		finishValidation({ valid: true });
		await tick();
		if (exit === 'Cancel') await expect.element(save).toHaveAttribute('aria-busy', 'false');
		expect(saved).not.toHaveBeenCalled();
		expect(mutation).not.toHaveBeenCalled();
		expect(onSubmit).not.toHaveBeenCalled();
		expect(errors.items).toHaveLength(0);
	}
);
