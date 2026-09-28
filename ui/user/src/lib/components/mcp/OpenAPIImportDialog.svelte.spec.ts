import { worker } from '../../../tests/mocks/worker';
import OpenAPIImportDialog from './OpenAPIImportDialog.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const result = {
	schema: { openapi: '3.1.0', info: { title: 'Example', version: '1' }, paths: {} },
	baseURL: '',
	suggestedHeaders: [],
	suggestedMetadata: { name: 'Example' }
};

it('keeps import errors in the modal and allows retry', async () => {
	let fail = true;
	worker.use(
		http.post('/api/mcp-catalogs/default/openapi/import', () => {
			if (fail) return new HttpResponse('Schema URL is unavailable', { status: 400 });
			return HttpResponse.json(result);
		})
	);
	const onComplete = vi.fn();
	await render(OpenAPIImportDialog, {
		id: 'default',
		entity: 'catalog',
		onComplete,
		onCancel: vi.fn()
	});
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/schema.json');
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent('400 /mcp-catalogs/default/openapi/import: Schema URL is unavailable');
	expect(onComplete).not.toHaveBeenCalled();
	fail = false;
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await vi.waitFor(() =>
		expect(onComplete).toHaveBeenCalledWith({
			config: { source: { url: 'https://example.com/schema.json' }, schema: result.schema },
			result
		})
	);
});

it('ignores a pending response after cancelling', async () => {
	let finish!: () => void;
	const pending = new Promise<void>((resolve) => {
		finish = resolve;
	});
	const responded = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/default/openapi/import', async () => {
			await pending;
			responded();
			return HttpResponse.json(result);
		})
	);
	const onComplete = vi.fn();
	const onCancel = vi.fn();
	const view = await render(OpenAPIImportDialog, {
		id: 'default',
		entity: 'catalog',
		onComplete,
		onCancel
	});
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/schema.json');
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect.element(page.getByLabelText('Schema URL', { exact: true })).toBeDisabled();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	expect(onCancel).toHaveBeenCalledOnce();
	await view.unmount();
	finish();
	await vi.waitFor(() => expect(responded).toHaveBeenCalledOnce());
	expect(onComplete).not.toHaveBeenCalled();
});

it('cancels when Escape closes the dialog', async () => {
	const onCancel = vi.fn();
	await render(OpenAPIImportDialog, {
		id: 'default',
		entity: 'catalog',
		onComplete: vi.fn(),
		onCancel
	});
	await expect.element(page.getByRole('dialog')).toBeVisible();
	await userEvent.keyboard('{Escape}');
	await vi.waitFor(() => expect(onCancel).toHaveBeenCalledOnce());
});
