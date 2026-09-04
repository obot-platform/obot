import VMcpGraph from './VMcpGraph.svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Item = { id: string };

const row = createRawSnippet((item: () => Item) => ({
	render: () => `<div style="width: 100px; height: 60px">Row for ${item().id}</div>`
}));

const empty = createRawSnippet(() => ({
	render: () => '<button type="button">Create New vMCP</button>'
}));

function renderGraph(item?: Item) {
	return render(VMcpGraph, {
		item,
		row,
		empty,
		estimateHeight: (_: Item, expanded: boolean) => (expanded ? 400 : 236)
	});
}

describe('VMcpGraph.svelte', () => {
	it('renders the selected vMCP on the canvas', async () => {
		renderGraph({ id: 'vmcp-1' });

		await expect.element(page.getByText('Row for vmcp-1')).toBeVisible();
		await expect.element(page.getByCSS('[data-vmcp-world]')).toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Create New vMCP' }))
			.not.toBeInTheDocument();
	});

	it('centers the selected vMCP in the canvas', async () => {
		renderGraph({ id: 'vmcp-1' });

		const canvas = await page.getByCSS('[data-vmcp-canvas]').element();
		// The canvas fills its container, which is content-sized here, so give it room to center in.
		(canvas.parentElement as HTMLElement).style.height = '600px';
		const world = await page.getByCSS('[data-vmcp-world]').element();
		await vi.waitFor(() => {
			const canvasBox = canvas.getBoundingClientRect();
			const worldBox = world.getBoundingClientRect();
			expect(worldBox.left + worldBox.width / 2).toBeCloseTo(
				canvasBox.left + canvasBox.width / 2,
				0
			);
			expect(worldBox.top + worldBox.height / 2).toBeCloseTo(
				canvasBox.top + canvasBox.height / 2,
				0
			);
		});
	});

	it('offers vMCP creation instead of a canvas when nothing is selected', async () => {
		renderGraph();

		await expect.element(page.getByRole('button', { name: 'Create New vMCP' })).toBeVisible();
		await expect.element(page.getByCSS('[data-vmcp-world]')).not.toBeInTheDocument();
		await expect.element(page.getByText('Row for vmcp-1')).not.toBeInTheDocument();
	});
});
