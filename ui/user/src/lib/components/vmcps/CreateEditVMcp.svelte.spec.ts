import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const sourceEntry = createMCPCatalogEntry({
	id: 'entry-gmail',
	name: 'Gmail',
	manifest: {
		toolPreview: [{ id: 'search_mail', name: 'search_mail', description: 'Search mail' }]
	}
});
const component = createVMCPComponent(sourceEntry, {
	id: 'component-gmail',
	name: 'Gmail',
	toolPrefix: 'gmail_',
	toolOverrides: [{ name: 'search_mail', description: 'Search mail', enabled: true }]
});
const vmcp = createVMCP({
	id: 'vmcp-1',
	displayName: 'Gmail vMCP',
	description: 'Mail tools',
	components: [component],
	profiles: [
		{
			name: 'mail readers',
			subjects: [{ type: 'user', id: 'user-1' }],
			allowAllTools: false,
			allowedTools: { 'component-gmail': ['search_mail'] }
		}
	]
});

async function renderEditor() {
	await preparePageData();
	return render(CreateEditVMcp);
}

function vmcpManifestFrom(requestBody: unknown) {
	return requestBody as {
		displayName: string;
		description?: string;
		icon?: string;
		components: typeof vmcp.components;
		profiles?: typeof vmcp.profiles;
		forceSingleUser?: boolean;
	};
}

describe('CreateEditVMcp.svelte', () => {
	beforeEach(() => {
		worker.use(
			http.get('/api/vmcps/:id', ({ params }) =>
				HttpResponse.json(params.id === vmcp.id ? vmcp : createVMCP({ id: String(params.id) }))
			)
		);
	});

	it('edits the flattened VMCP metadata and profiles without loading legacy access policies', async () => {
		const legacyAccessRules = vi.fn();
		worker.use(
			http.get('/api/mcp-catalogs/default/access-control-rules', () => {
				legacyAccessRules();
				return HttpResponse.json({ items: [] });
			}),
			http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) =>
				HttpResponse.json({ ...vmcp, ...vmcpManifestFrom(await request.json()) })
			)
		);

		const result = await renderEditor();
		await result.component.openEdit(vmcp);

		await expect.element(page.getByRole('textbox', { name: 'Name' })).toHaveValue(vmcp.displayName);
		await expect
			.element(page.getByRole('textbox', { name: 'Description' }))
			.toHaveValue(vmcp.description!);
		expect(legacyAccessRules).not.toHaveBeenCalled();
	});

	it('creates a vMCP through the first-class endpoint with a cached component snapshot', async () => {
		const createRequest = vi.fn();
		const legacyCreate = vi.fn();
		worker.use(
			http.post('/api/vmcps', async ({ request }) => {
				const manifest = await request.json();
				createRequest(manifest);
				return HttpResponse.json(createVMCP({ id: 'vmcp-created', ...vmcpManifestFrom(manifest) }));
			}),
			http.post('/api/mcp-catalogs/default/entries', () => {
				legacyCreate();
				return HttpResponse.json({});
			})
		);

		const result = await renderEditor();
		result.component.openCreate([component]);

		await expect
			.element(page.getByRole('textbox', { name: 'Name' }))
			.toHaveValue(sourceEntry.manifest.name!);
		await expect
			.element(page.getByRole('textbox', { name: 'Description' }))
			.toHaveValue(sourceEntry.manifest.shortDescription!);

		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(createRequest).toHaveBeenCalledOnce());

		const manifest = vmcpManifestFrom(createRequest.mock.calls[0][0]);
		expect(manifest).toMatchObject({
			displayName: sourceEntry.manifest.name,
			components: [
				{
					id: component.id,
					name: component.name,
					mcpCatalogID: 'default',
					mcpServerCatalogEntryID: sourceEntry.id,
					catalogEntry: {
						manifest: expect.objectContaining({
							name: sourceEntry.manifest.name,
							runtime: sourceEntry.manifest.runtime,
							toolPreview: sourceEntry.manifest.toolPreview
						}),
						unsupportedTools: []
					}
				}
			]
		});
		expect(manifest.components[0]).not.toHaveProperty('mcpServerID');
		expect(manifest).toHaveProperty('profiles');
		expect(manifest).toHaveProperty('forceSingleUser', false);
		expect(legacyCreate).not.toHaveBeenCalled();
	});

	it('creates a vMCP without components', async () => {
		const createRequest = vi.fn();
		worker.use(
			http.post('/api/vmcps', async ({ request }) => {
				const manifest = await request.json();
				createRequest(manifest);
				return HttpResponse.json(createVMCP({ id: 'vmcp-empty', ...vmcpManifestFrom(manifest) }));
			})
		);
		const result = await renderEditor();
		result.component.openCreate([]);
		await page.getByRole('textbox', { name: 'Name' }).fill('Empty draft');
		await page.getByRole('textbox', { name: 'Description' }).fill('Configure later');
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(createRequest).toHaveBeenCalledOnce());
		expect(createRequest.mock.calls[0][0]).toMatchObject({ components: [] });
	});

	it('updates a vMCP manifest while preserving immutable component IDs and profile tool policy', async () => {
		const updateRequest = vi.fn();
		const legacyUpdate = vi.fn();
		worker.use(
			http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) => {
				const manifest = await request.json();
				updateRequest(manifest);
				return HttpResponse.json({ ...vmcp, ...vmcpManifestFrom(manifest) });
			}),
			http.put(`/api/mcp-catalogs/default/entries/${vmcp.id}`, () => {
				legacyUpdate();
				return HttpResponse.json({});
			})
		);

		const result = await renderEditor();
		await result.component.openEdit(vmcp);
		await page.getByRole('textbox', { name: 'Name' }).fill('Mail Gateway');
		await page.getByRole('button', { name: 'Save changes', exact: true }).click();

		await vi.waitFor(() => expect(updateRequest).toHaveBeenCalledOnce());
		const manifest = vmcpManifestFrom(updateRequest.mock.calls[0][0]);
		expect(manifest).toMatchObject({
			displayName: 'Mail Gateway',
			components: [{ id: 'component-gmail', mcpServerCatalogEntryID: sourceEntry.id }],
			profiles: vmcp.profiles,
			forceSingleUser: false
		});
		expect(manifest.components[0].catalogEntry.manifest).toEqual(component.catalogEntry.manifest);
		expect(legacyUpdate).not.toHaveBeenCalled();
	});

	it('deletes a vMCP through the first-class endpoint', async () => {
		const deleteRequest = vi.fn();
		const legacyDelete = vi.fn();
		worker.use(
			http.delete(`/api/vmcps/${vmcp.id}`, () => {
				deleteRequest();
				return new HttpResponse(null, { status: 204 });
			}),
			http.delete(`/api/mcp-catalogs/default/entries/${vmcp.id}`, () => {
				legacyDelete();
				return new HttpResponse(null, { status: 204 });
			})
		);

		const result = await renderEditor();
		await result.component.openEdit(vmcp);
		await page.getByRole('button', { name: /Delete/ }).click();
		await expect.element(page.getByText('Confirm Delete')).toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleteRequest).toHaveBeenCalledOnce());
		expect(legacyDelete).not.toHaveBeenCalled();
	});
});
