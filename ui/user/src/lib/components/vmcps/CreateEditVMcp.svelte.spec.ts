import type { MCPCatalogEntryServerManifest } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const vmcp = createMCPCatalogEntry({
	id: 'vmcp-1',
	name: 'Gmail vMCP'
});

async function renderEditor() {
	await preparePageData();
	return render(CreateEditVMcp);
}

describe('CreateEditVMcp.svelte', () => {
	it('shows assigned access policies when editing a vMCP', async () => {
		worker.use(
			http.get('/api/mcp-catalogs/default/access-control-rules', () =>
				HttpResponse.json({
					items: [
						{
							id: 'policy-assigned',
							created: '2026-01-01T00:00:00Z',
							displayName: 'Assigned Policy',
							subjects: [{ type: 'user', id: 'user-1' }],
							resources: [{ type: 'mcpServerCatalogEntry', id: vmcp.id }]
						},
						{
							id: 'policy-other',
							created: '2026-01-01T00:00:00Z',
							displayName: 'Other Policy',
							subjects: [],
							resources: []
						}
					]
				})
			)
		);

		const result = await renderEditor();
		await result.component.openEdit(vmcp);

		await expect.element(page.getByText('Assigned Policy', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Other Policy', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add access policy' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Remove access policy' })).toBeVisible();
	});

	it('does not allow removing a global access policy', async () => {
		worker.use(
			http.get('/api/mcp-catalogs/default/access-control-rules', () =>
				HttpResponse.json({
					items: [
						{
							id: 'policy-assigned',
							created: '2026-01-01T00:00:00Z',
							displayName: 'Assigned Policy',
							subjects: [{ type: 'user', id: 'user-1' }],
							resources: [{ type: 'mcpServerCatalogEntry', id: vmcp.id }]
						},
						{
							id: 'policy-global',
							created: '2026-01-01T00:00:00Z',
							displayName: 'Global Policy',
							subjects: [{ type: 'selector', id: '*' }],
							resources: [{ type: 'selector', id: '*' }]
						}
					]
				})
			)
		);

		const result = await renderEditor();
		await result.component.openEdit(vmcp);

		await expect.element(page.getByText('Assigned Policy', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Global Policy', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Remove access policy' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Remove access policy' }).nth(1))
			.not.toBeInTheDocument();
	});

	it('creates a vMCP with the supplied component server and its prefilled details', async () => {
		const entry = createMCPCatalogEntry({ id: 'entry-gmail', name: 'Gmail' });
		const createRequest = vi.fn();

		worker.use(
			http.post('/api/mcp-catalogs/default/entries', async ({ request }) => {
				const manifest = (await request.json()) as MCPCatalogEntryServerManifest;
				createRequest(manifest);
				return HttpResponse.json({ ...vmcp, manifest });
			})
		);

		const result = await renderEditor();
		result.component.openCreate([{ catalogEntryID: entry.id, manifest: entry.manifest }]);

		await expect
			.element(page.getByRole('textbox', { name: 'Name' }))
			.toHaveValue(entry.manifest.name!);
		await expect
			.element(page.getByRole('textbox', { name: 'Description' }))
			.toHaveValue(entry.manifest.shortDescription!);

		await page.getByRole('button', { name: 'Create' }).click();

		await vi.waitFor(() => expect(createRequest).toHaveBeenCalled());
		expect(createRequest.mock.calls[0][0].compositeConfig.componentServers).toEqual([
			{ catalogEntryID: entry.id, manifest: entry.manifest }
		]);
	});
});
