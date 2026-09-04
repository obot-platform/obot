import type { VMCPManifest } from '$lib/services';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const vmcp = createVMCP({
	id: 'vmcp-1',
	displayName: 'Gmail vMCP',
	description: 'Gmail vMCP short description'
});

async function renderEditor() {
	await preparePageData();
	return render(CreateEditVMcp);
}

describe('CreateEditVMcp.svelte', () => {
	it('opens the edit dialog with name and description only', async () => {
		const result = await renderEditor();
		result.component.openEdit(vmcp);

		await expect.element(page.getByRole('textbox', { name: 'Name' })).toHaveValue(vmcp.displayName);
		await expect
			.element(page.getByRole('textbox', { name: 'Description' }))
			.toHaveValue(vmcp.description!);
		await expect.element(page.getByText('Access Policies')).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add access policy' }))
			.not.toBeInTheDocument();
	});

	it('creates a vMCP with the supplied component server and its prefilled details', async () => {
		const entry = createMCPCatalogEntry({ id: 'entry-gmail', name: 'Gmail' });
		const createRequest = vi.fn();

		worker.use(
			http.post('/api/vmcps', async ({ request }) => {
				const manifest = (await request.json()) as VMCPManifest;
				createRequest(manifest);
				return HttpResponse.json({ ...vmcp, ...manifest });
			})
		);

		const result = await renderEditor();
		result.component.openCreate([createVMCPComponent(entry)]);

		await expect
			.element(page.getByRole('textbox', { name: 'Name' }))
			.toHaveValue(entry.manifest.name!);
		await expect
			.element(page.getByRole('textbox', { name: 'Description' }))
			.toHaveValue(entry.manifest.shortDescription!);

		await page.getByRole('button', { name: 'Create' }).click();

		await vi.waitFor(() => expect(createRequest).toHaveBeenCalled());
		expect(createRequest.mock.calls[0][0].components).toMatchObject([
			{
				mcpServerCatalogEntryID: entry.id,
				name: entry.manifest.name
			}
		]);
	});
});
