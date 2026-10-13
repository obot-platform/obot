import type { VMCPComponent, VMCPManifest } from '$lib/services';
import { catalogEntryToVMCPComponent } from '$lib/services/vmcps/utils';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
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

function componentWithTools(count: number, name = 'tools'): VMCPComponent {
	return catalogEntryToVMCPComponent(
		createMCPCatalogEntry({
			id: `entry-${name}`,
			name,
			manifest: {
				shortDescription: 'Tools for testing',
				toolPreview: Array.from({ length: count }, (_, index) => ({
					id: `${name}-${index}`,
					name: `${name}_${index}`
				}))
			}
		})
	);
}

async function renderCreationWithTools(components: VMCPComponent[], failFirst = false) {
	await preparePageData();
	const create = vi.fn();
	const onCreated = vi.fn();
	worker.use(
		http.post('/api/vmcps', async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			create(manifest);
			if (failFirst && create.mock.calls.length === 1) {
				return new HttpResponse(null, { status: 500 });
			}
			return HttpResponse.json(createVMCP(manifest));
		})
	);
	const result = await render(CreateEditVMcp, { onCreated });
	result.component.openCreate(components);
	await page.getByRole('textbox', { name: 'Name' }).fill('Large vMCP');
	await page.getByRole('textbox', { name: 'Description' }).fill('Many tools');
	return { result, create, onCreated };
}

describe('large vMCP tool search recommendation', () => {
	it.each([
		{ choice: 'Enable tool search', enabled: true },
		{ choice: 'Keep direct tools', enabled: false }
	])('creates with the selected mode after choosing $choice', async ({ choice, enabled }) => {
		const { create, onCreated } = await renderCreationWithTools([
			componentWithTools(30, 'first'),
			componentWithTools(21, 'second')
		]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await expect.element(page.getByRole('heading', { name: 'Enable tool search?' })).toBeVisible();
		await expect.element(page.getByText(/Based on its tool previews/)).toBeVisible();
		expect(create).not.toHaveBeenCalled();
		await page.getByRole('button', { name: choice, exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create).toHaveBeenCalledOnce();
		expect(create.mock.calls[0][0]).toMatchObject({
			displayName: 'Large vMCP',
			description: 'Many tools',
			toolSearch: enabled
		});
		await expect
			.element(page.getByRole('heading', { name: 'Enable tool search?' }))
			.not.toBeInTheDocument();
	});

	it.each([0, 49, 50])('creates without prompting for %s tools', async (count) => {
		const { create, onCreated } = await renderCreationWithTools([componentWithTools(count)]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create.mock.calls[0][0].toolSearch).toBe(false);
		await expect
			.element(page.getByRole('heading', { name: 'Enable tool search?' }))
			.not.toBeInTheDocument();
	});

	it('counts enabled overrides instead of all preview tools', async () => {
		const component = componentWithTools(100);
		component.toolOverrides = component.catalogEntry.manifest.toolPreview!.map((tool, index) => ({
			name: tool.name,
			enabled: index < 50
		}));
		const { create, onCreated } = await renderCreationWithTools([component]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create.mock.calls[0][0].toolSearch).toBe(false);
	});

	it('prompts when explicit selections exceed 50 enabled tools', async () => {
		const component = componentWithTools(100);
		component.toolOverrides = component.catalogEntry.manifest.toolPreview!.map((tool, index) => ({
			name: tool.name,
			enabled: index < 51
		}));
		const { create, onCreated } = await renderCreationWithTools([component]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await expect.element(page.getByRole('heading', { name: 'Enable tool search?' })).toBeVisible();
		await expect.element(page.getByText(/This vMCP has more than 50 enabled tools/)).toBeVisible();
		expect(create).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Enable tool search', exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create.mock.calls[0][0].toolSearch).toBe(true);
	});

	it('skips the prompt when tool search is already selected', async () => {
		const { create, onCreated } = await renderCreationWithTools([componentWithTools(51)]);
		await page.getByRole('checkbox', { name: 'Tool search' }).click();
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create.mock.calls[0][0].toolSearch).toBe(true);
		await expect
			.element(page.getByRole('heading', { name: 'Enable tool search?' }))
			.not.toBeInTheDocument();
	});

	it('goes back without creating and resets the prompt for another vMCP', async () => {
		const { result, create, onCreated } = await renderCreationWithTools([componentWithTools(51)]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await page.getByRole('button', { name: 'Back', exact: true }).click();
		await expect.element(page.getByRole('textbox', { name: 'Name' })).toHaveValue('Large vMCP');
		await expect.element(page.getByRole('checkbox', { name: 'Tool search' })).not.toBeChecked();
		expect(create).not.toHaveBeenCalled();
		result.component.openCreate([componentWithTools(50)]);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create.mock.calls[0][0].toolSearch).toBe(false);
	});

	it('allows retrying a failed creation with the selected mode', async () => {
		const { create, onCreated } = await renderCreationWithTools([componentWithTools(51)], true);
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		const enable = page.getByRole('button', { name: 'Enable tool search', exact: true });
		await enable.click();
		await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());
		await expect.element(enable).toBeEnabled();
		expect(onCreated).not.toHaveBeenCalled();
		await enable.click();
		await vi.waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
		expect(create).toHaveBeenCalledTimes(2);
		expect(create.mock.calls.every(([manifest]) => manifest.toolSearch === true)).toBe(true);
	});
});
