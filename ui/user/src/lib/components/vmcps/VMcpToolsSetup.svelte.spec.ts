import { createMCPCatalogEntry, createVMCPComponent } from '../../../tests/helpers/mcp';
import VMcpToolsSetup from './VMcpToolsSetup.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const component = createVMCPComponent(
	createMCPCatalogEntry({
		id: 'entry-tools-setup',
		name: 'Tools Setup Server',
		manifest: {
			toolPreview: [
				{ id: 'search', name: 'search', description: 'Search things' },
				{ id: 'list', name: 'list', description: 'List things' }
			]
		}
	}),
	{ id: 'component-tools-setup' }
);

function createTools() {
	return [
		{
			id: 'component-tools-setup-search',
			name: 'search',
			description: 'Search things',
			enabled: true
		},
		{
			id: 'component-tools-setup-list',
			name: 'list',
			description: 'List things',
			enabled: false
		}
	];
}

async function waitForNativeClose(locator: ReturnType<typeof page.getByRole>) {
	const element = await locator.element();
	if (!(element instanceof HTMLDialogElement)) {
		throw new Error('Expected a native dialog element');
	}

	return new Promise<void>((resolve) => {
		element.addEventListener('close', () => resolve(), { once: true });
	});
}

function setupDialog() {
	return page
		.getByRole('dialog')
		.filter({ hasText: 'Tools are read from the catalog-entry snapshot stored on this vMCP.' });
}

function editorDialog() {
	return page.getByRole('dialog').filter({ hasText: 'Tool name prefix' });
}

describe('VMcpToolsSetup', () => {
	it('keeps the editor open after the setup dialog finishes its native close animation', async () => {
		const onSuccess = vi.fn();
		const result = await render(VMcpToolsSetup, {
			component,
			existingTools: createTools(),
			onSuccess
		});

		await result.component.open();
		await expect.element(setupDialog()).toBeVisible();
		const setupClosed = waitForNativeClose(setupDialog());

		await page.getByRole('button', { name: 'Configure Tools', exact: true }).click();
		await setupClosed;

		await expect.element(editorDialog()).toBeVisible();

		const prefix = editorDialog().getByCSS('input[placeholder="No prefix"]');
		await expect.element(prefix).toHaveValue('');
		await prefix.fill('team_');
		await editorDialog().getByRole('checkbox', { name: 'Enabled' }).nth(1).click();
		await editorDialog().getByRole('button', { name: 'Confirm', exact: true }).click();

		await vi.waitFor(() => expect(onSuccess).toHaveBeenCalledOnce());
		expect(onSuccess).toHaveBeenCalledWith(
			expect.objectContaining({
				toolPrefix: 'team_',
				toolOverrides: expect.arrayContaining([
					expect.objectContaining({ name: 'search', enabled: true }),
					expect.objectContaining({ name: 'list', enabled: true })
				])
			})
		);
	});

	it('calls explicit cancellation once after the setup dialog closes', async () => {
		const onCancel = vi.fn();
		const result = await render(VMcpToolsSetup, {
			component,
			existingTools: createTools(),
			onCancel
		});

		await result.component.open();
		await expect.element(setupDialog()).toBeVisible();
		const setupClosed = waitForNativeClose(setupDialog());

		await page.getByRole('button', { name: "Skip, I'll Do Later", exact: true }).click();
		await setupClosed;

		await vi.waitFor(() => expect(onCancel).toHaveBeenCalledOnce());
		await expect.element(page.getByCSS('dialog[open]')).not.toBeInTheDocument();
	});
});
