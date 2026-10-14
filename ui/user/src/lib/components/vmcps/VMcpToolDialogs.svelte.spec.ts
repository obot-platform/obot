import { createVMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import type { VMCP, VMCPManifest } from '$lib/services';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpToolDialogs from './VMcpToolDialogs.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderLiveToolSelection(
	count: number,
	options: {
		toolSearch?: boolean;
		otherCount?: number;
		otherPreviewCount?: number;
		stripPreviews?: boolean;
		failEnable?: boolean;
		failSave?: boolean;
	} = {}
) {
	await preparePageData();
	const entry = createMCPCatalogEntry({
		id: 'entry-live',
		name: 'Live server',
		runtime: 'remote',
		manifest: { toolPreview: [] }
	});
	const component = createVMCPComponent(entry);
	const otherEntry = createMCPCatalogEntry({
		id: 'entry-other',
		name: 'Other server',
		manifest: {
			toolPreview: Array.from({ length: options.otherPreviewCount ?? 0 }, (_, index) => ({
				id: `other-${index}`,
				name: `other_${index}`
			}))
		}
	});
	const other = createVMCPComponent(otherEntry, {
		toolOverrides: Array.from({ length: options.otherCount ?? 0 }, (_, index) => ({
			name: `other_${index}`,
			enabled: true
		}))
	});
	if (options.otherPreviewCount !== undefined) other.toolOverrides = undefined;
	let current = createVMCP({
		id: 'vmcp-live',
		toolSearch: options.toolSearch ?? false,
		components:
			options.otherCount || options.otherPreviewCount !== undefined
				? [component, other]
				: [component]
	});
	function apiSnapshot() {
		const snapshot = structuredClone(current);
		if (options.stripPreviews) {
			for (const component of snapshot.components) {
				delete component.catalogEntry.manifest.toolPreview;
			}
		}
		return snapshot;
	}
	const liveEntry = {
		...entry,
		manifest: {
			...entry.manifest,
			toolPreview: Array.from({ length: count }, (_, index) => ({
				id: `tool-${index}`,
				name: `tool_${index}`
			}))
		}
	};
	const update = vi.fn();
	const preview = vi.fn();
	worker.use(
		http.get(`/api/vmcps/${current.id}`, () => HttpResponse.json(apiSnapshot())),
		http.post(`/api/vmcps/${current.id}/components/${other.id}/generate-tool-previews`, () => {
			preview();
			return HttpResponse.json(otherEntry);
		}),
		http.post(`/api/vmcps/${current.id}/components/${component.id}/generate-tool-previews`, () =>
			HttpResponse.json(liveEntry)
		),
		http.put(`/api/vmcps/${current.id}`, async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			update(manifest);
			if (
				(options.failSave && update.mock.calls.length === 1) ||
				(options.failEnable && manifest.toolSearch && update.mock.calls.length === 2)
			) {
				return new HttpResponse(null, { status: 500 });
			}
			current = { ...current, ...manifest };
			return HttpResponse.json(apiSnapshot());
		})
	);
	const flow = createVMcpToolFlow();
	const changed = vi.fn();
	flow.setOnVMcpChanged(changed);
	render(VMcpToolDialogs, { flow });
	flow.handleVMcpCreated(apiSnapshot());
	await page.getByRole('button', { name: /^Managed/ }).click();
	await page.getByRole('button', { name: /Configure Tools/i, exact: true }).click();
	await expect
		.element(page.getByRole('switch', { name: 'Enable All Tools', exact: true }))
		.toBeChecked();
	return {
		flow,
		update,
		changed,
		preview,
		setLatest: (changes: Partial<VMCP>) => {
			current = { ...current, ...changes };
		}
	};
}

it('offers tool search after discovering and selecting more than 50 live tools', async () => {
	const { update, changed, setLatest } = await renderLiveToolSelection(51);
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	expect(
		update.mock.calls[0][0].components[0].toolOverrides.filter(
			(tool: { enabled: boolean }) => tool.enabled
		)
	).toHaveLength(51);
	await expect.element(page.getByText('Enable tool search?', { exact: true })).toBeVisible();
	setLatest({ description: 'Edited elsewhere while the prompt was open' });
	await page.getByRole('button', { name: 'Enable tool search', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(2));
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
	expect(update.mock.calls[1][0]).toMatchObject({
		toolSearch: true,
		description: 'Edited elsewhere while the prompt was open'
	});
	expect(update.mock.calls[1][0].components[0].toolOverrides).toHaveLength(51);
	expect(changed.mock.calls.at(-1)?.[0].toolSearch).toBe(true);
});

it('keeps the selected tools without enabling search and does not ask again in the same flow', async () => {
	const { flow, update, changed } = await renderLiveToolSelection(51);
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await page.getByRole('button', { name: 'Keep direct tools', exact: true }).click();
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
	expect(update).toHaveBeenCalledOnce();
	const saved = changed.mock.calls.at(-1)?.[0] as VMCP;
	expect(saved.toolSearch).toBe(false);
	expect(saved.components[0].toolOverrides).toHaveLength(51);
	await expect
		.element(page.getByRole('button', { name: 'Confirm', exact: true }))
		.not.toBeInTheDocument();
	flow.openEdit(saved, saved.components[0]);
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(2));
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
});

it('does not recommend search when only 50 of 51 discovered tools are enabled', async () => {
	const { update } = await renderLiveToolSelection(51);
	await page.getByRole('switch', { name: 'Disable Tool', exact: true }).nth(50).click();
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	expect(
		update.mock.calls[0][0].components[0].toolOverrides.filter(
			(tool: { enabled: boolean }) => tool.enabled
		)
	).toHaveLength(50);
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
});

it('does not recommend search when it is already enabled', async () => {
	const { update } = await renderLiveToolSelection(51, { toolSearch: true });
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
});

it('counts enabled tools across components after selection', async () => {
	const { update } = await renderLiveToolSelection(21, { otherCount: 30 });
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	await expect.element(page.getByText('Enable tool search?', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Enable tool search', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(2));
	expect(update.mock.calls[1][0].components[1].toolOverrides).toHaveLength(30);
	expect(update.mock.calls[1][0].toolSearch).toBe(true);
});

it('keeps the prompt open and preserves selections if enabling fails', async () => {
	const { update } = await renderLiveToolSelection(51, { failEnable: true });
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	const enable = page.getByRole('button', { name: 'Enable tool search', exact: true });
	await enable.click();
	await expect.element(page.getByRole('alert')).toBeVisible();
	await expect.element(enable).toBeEnabled();
	await enable.click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(3));
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
	expect(update.mock.calls[2][0].toolSearch).toBe(true);
	expect(update.mock.calls[2][0].components[0].toolOverrides).toHaveLength(51);
});

it.each([
	{ count: 20, recommends: false },
	{ count: 50, recommends: false },
	{ count: 51, recommends: true }
])(
	'saves $count selected tools without generating missing component previews',
	async ({ count, recommends }) => {
		const { preview, update, changed } = await renderLiveToolSelection(count, {
			otherPreviewCount: 40,
			stripPreviews: true
		});
		await page.getByRole('button', { name: 'Confirm', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Confirm', exact: true }))
			.not.toBeInTheDocument();
		expect(preview).not.toHaveBeenCalled();
		expect(update).toHaveBeenCalledOnce();
		expect(changed).toHaveBeenCalledOnce();
		expect(changed.mock.calls[0][0].components[0].toolOverrides).toHaveLength(count);
		expect(
			changed.mock.calls[0][0].components[1].catalogEntry.manifest.toolPreview
		).toBeUndefined();
		const prompt = page.getByText('Enable tool search?', { exact: true });
		if (recommends) {
			await expect.element(prompt).toBeVisible();
			await page.getByRole('button', { name: 'Enable tool search', exact: true }).click();
			await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(2));
			expect(update.mock.calls[1][0].toolSearch).toBe(true);
			expect(update.mock.calls[1][0].components[0].toolOverrides).toHaveLength(count);
			expect(update.mock.calls[1][0].components[1].toolOverrides).toBeUndefined();
			expect(preview).not.toHaveBeenCalled();
		} else {
			await expect.element(prompt).not.toBeVisible();
		}
	}
);

it('does not offer search if saving the selected tools failed', async () => {
	const { update } = await renderLiveToolSelection(51, { failSave: true });
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
	await expect.element(page.getByText('Enable tool search?', { exact: true })).not.toBeVisible();
});
