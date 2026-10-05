import type { VMCPManifest } from '$lib/services';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('creates a vMCP with tool search when selected', async () => {
	await preparePageData();
	const create = vi.fn();
	worker.use(
		http.post('/api/vmcps', async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			create(manifest);
			return HttpResponse.json(createVMCP(manifest));
		})
	);
	const result = await render(CreateEditVMcp);
	result.component.openCreate();
	await page.getByRole('textbox', { name: 'Name' }).fill('Search vMCP');
	await page.getByRole('textbox', { name: 'Description' }).fill('Search enabled');
	const toggle = page.getByRole('checkbox', { name: /Tool search/ });
	await expect.element(toggle).not.toBeChecked();
	await expect.element(toggle).toHaveClass(/toggle/);
	await expect
		.element(page.getByCSS('label[for="vmcp-tool-search"]'))
		.toHaveClass(/justify-between/);
	await toggle.click();
	await page.getByRole('button', { name: 'Create', exact: true }).click();
	await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());
	expect(create.mock.calls[0][0].toolSearch).toBe(true);
	await vi.waitFor(() => expect(page.getByCSS('dialog[open]').all()).toHaveLength(0));
});

it('preserves and updates tool search when editing a vMCP', async () => {
	await preparePageData();
	const vmcp = createVMCP({ toolSearch: true });
	const update = vi.fn();
	worker.use(
		http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			update(manifest);
			return HttpResponse.json(createVMCP({ ...vmcp, ...manifest }));
		})
	);
	const result = await render(CreateEditVMcp);
	result.component.openEdit(vmcp);
	await expect.element(page.getByRole('dialog')).toBeVisible();
	const toggle = page.getByRole('checkbox', { name: /Tool search/ });
	await expect.element(toggle).toBeChecked();
	await expect.element(toggle).toBeEnabled();
	await toggle.click();
	await expect.element(toggle).not.toBeChecked();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	expect(update.mock.calls[0][0].toolSearch).toBe(false);
});

it('shows catalog synced tool search as read only', async () => {
	await preparePageData();
	const vmcp = createVMCP({ toolSearch: true, sourceURL: 'https://example.com/catalog' });
	const result = await render(CreateEditVMcp);
	result.component.openEdit(vmcp);
	const toggle = page.getByRole('checkbox', { name: /Tool search/ });
	await expect.element(toggle).toBeChecked();
	await expect.element(toggle).toBeDisabled();
});
