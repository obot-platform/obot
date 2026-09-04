import VMcpDragHint from './VMcpDragHint.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const STORAGE_KEY = '@obot/seen-vmcp-drag-hint';

function caption() {
	return page.getByText('Drag a server from the panel anywhere onto the canvas to build a vMCP.');
}

function dismissButton() {
	return page.getByRole('button', { name: 'Dismiss drag and drop tip' });
}

describe('VMcpDragHint.svelte', () => {
	it('shows the tip on a first visit', async () => {
		render(VMcpDragHint);

		await expect.element(caption()).toBeVisible();
		await expect.element(page.getByText('Create New vMCP')).toBeVisible();
		await expect.element(page.getByText('MCP Server')).toBeVisible();
	});

	it('stays hidden once it has been seen', async () => {
		localStorage.setItem(STORAGE_KEY, new Date().toISOString());
		render(VMcpDragHint);

		await expect.element(caption()).not.toBeInTheDocument();
	});

	it('remembers a dismissal so it does not come back', async () => {
		render(VMcpDragHint);

		await dismissButton().click();

		await expect.element(caption()).not.toBeInTheDocument();
		expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull();
	});

	it('retires itself once the user drags for real', async () => {
		render(VMcpDragHint, { dragActive: true });

		await expect.element(caption()).not.toBeInTheDocument();
		expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull();
	});
});
