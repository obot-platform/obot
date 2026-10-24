import TesterError from './TesterError.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

describe('TesterError', () => {
	it.each([
		{ name: 'plain text', message: 'Gateway failed\nPlease retry.', details: undefined },
		{
			name: 'JSON text',
			message: '{"code":-32600,"message":"Invalid output"}',
			details: undefined
		},
		{
			name: 'structured details',
			message: 'MCP error',
			details: { code: -32600, message: 'Invalid output' }
		}
	])('copies the complete displayed error for $name', async ({ message, details }) => {
		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
		try {
			render(TesterError, { title: 'Request failed', message, details });
			await page.getByRole('button', { name: 'View full error' }).click();
			const { EditorView } = await import('@codemirror/view');
			const displayedError = EditorView.findFromDOM(
				page.getByLabelText('Full error').element() as HTMLElement
			)?.state.doc.toString();
			await page.getByRole('button', { name: 'Copy error', exact: true }).click();
			expect(writeText).toHaveBeenCalledWith(displayedError);
		} finally {
			writeText.mockRestore();
		}
	});

	it('omits the raw response section when no response was received', async () => {
		render(TesterError, { title: 'Request failed', message: 'Network unavailable' });
		await page.getByRole('button', { name: 'View full error' }).click();
		await expect.element(page.getByText('Raw response', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByLabelText('Full error')).toBeVisible();
	});

	it.each([
		{ width: 1280, height: 720 },
		{ width: 390, height: 844 }
	])(
		'bounds a large error and response at $width px while keeping controls reachable',
		async ({ width, height }) => {
			await page.viewport(width, height);
			try {
				render(TesterError, {
					title: 'Request failed',
					message: 'Validation failed',
					details: Array.from({ length: 100 }, (_, i) => ({ error: `Invalid field ${i}` })),
					rawResponse: {
						jsonrpc: '2.0',
						id: 3,
						result: {
							content: Array.from({ length: 200 }, (_, i) => ({
								type: 'text',
								text: `Response ${i}`
							}))
						}
					}
				});
				await page.getByRole('button', { name: 'View full error' }).click();
				await page.getByText('Raw response', { exact: true }).click();
				await expect.element(page.getByLabelText('Raw MCP response')).toBeVisible();
				const dialog = page.getByRole('dialog');
				expect(dialog.element().getBoundingClientRect().height).toBeLessThanOrEqual(
					window.innerHeight
				);
				const scroller = page
					.getByCSS('[aria-label="Raw MCP response"] .cm-scroller')
					.element() as HTMLElement;
				expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight);
				await dialog
					.getByRole('button', { name: /^close$/i })
					.first()
					.click();
				await expect.element(page.getByRole('dialog', { includeHidden: true })).not.toBeVisible();
			} finally {
				await page.viewport(1280, 720);
			}
		}
	);

	it('lazily expands, copies, and resets the raw response while keeping error details visible', async () => {
		const rawResponse = {
			jsonrpc: '2.0',
			id: 3,
			result: { content: [{ type: 'text', text: 'no_subscription' }] }
		};
		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
		render(TesterError, {
			title: 'Request failed',
			message: 'Output schema validation failed',
			rawResponse
		});
		const trigger = page.getByRole('button', { name: 'View full error' });
		await trigger.click();
		await expect.element(page.getByLabelText('Full error')).toBeVisible();
		await expect.element(page.getByLabelText('Raw MCP response')).not.toBeInTheDocument();
		const summary = page.getByText('Raw response', { exact: true });
		(summary.element() as HTMLElement).focus();
		await userEvent.keyboard('{Enter}');
		const preview = page.getByLabelText('Raw MCP response');
		await expect.element(preview).toBeVisible();
		const { EditorView } = await import('@codemirror/view');
		expect(EditorView.findFromDOM(preview.element() as HTMLElement)?.state.doc.toString()).toBe(
			JSON.stringify(rawResponse, null, 2)
		);
		await page.getByRole('button', { name: 'Copy raw JSON' }).click();
		expect(writeText).toHaveBeenCalledWith(JSON.stringify(rawResponse, null, 2));
		await summary.click();
		await expect.element(preview).not.toBeInTheDocument();
		await summary.click();
		await userEvent.keyboard('{Escape}');
		await expect.element(trigger).toHaveFocus();
		await trigger.click();
		await expect.element(summary).toBeVisible();
		await expect.element(preview).not.toBeInTheDocument();
		writeText.mockRestore();
	});

	it('bounds the preview, lazily pretty-prints the entire error, and restores focus after Escape', async () => {
		const errors = Array.from({ length: 100 }, (_, index) => ({
			path: ['tools', index, 'outputSchema', 'type'],
			message: `Invalid schema ${index}`
		}));
		render(TesterError, { title: 'Tools could not be loaded', message: JSON.stringify(errors) });
		const alert = page.getByRole('alert');
		expect(alert.element().textContent?.length).toBeLessThan(500);
		await expect.element(page.getByLabelText('Full error')).not.toBeInTheDocument();
		const button = page.getByRole('button', { name: 'View full error' });
		await button.click();
		await expect.element(page.getByLabelText('Full error')).toBeVisible();
		const editor = page.getByLabelText('Full error');
		const view = await import('@codemirror/view');
		const editorView = view.EditorView.findFromDOM(editor.element() as HTMLElement);
		expect(editorView?.state.doc.toString()).toBe(JSON.stringify(errors, null, 2));
		const scroller = page
			.getByCSS('[aria-label="Full error"] .cm-scroller')
			.element() as HTMLElement;
		expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight);
		expect(page.getByRole('dialog').element().getBoundingClientRect().height).toBeLessThanOrEqual(
			window.innerHeight
		);
		await userEvent.keyboard('{Escape}');
		await expect.element(page.getByRole('dialog', { includeHidden: true })).not.toBeVisible();
		await expect.element(button).toHaveFocus();
		await expect.element(editor).not.toBeInTheDocument();
	});

	it('displays plain text unchanged and closes using the existing dialog control', async () => {
		const message = 'Gateway failed\nPlease retry.';
		render(TesterError, { title: 'Request failed', message });
		await page.getByRole('button', { name: 'View full error' }).click();
		const { EditorView } = await import('@codemirror/view');
		expect(
			EditorView.findFromDOM(
				page.getByLabelText('Full error').element() as HTMLElement
			)?.state.doc.toString()
		).toBe(message);
		await page
			.getByRole('dialog')
			.getByRole('button', { name: /^close$/i })
			.first()
			.click();
		await expect.element(page.getByRole('dialog', { includeHidden: true })).not.toBeVisible();
	});

	it('uses structured details instead of truncating them to the summary', async () => {
		const details = { code: -32602, data: { reason: 'invalid parameters' } };
		render(TesterError, { title: 'Request failed', message: 'MCP error', details });
		await page.getByRole('button', { name: 'View full error' }).click();
		const { EditorView } = await import('@codemirror/view');
		expect(
			EditorView.findFromDOM(
				page.getByLabelText('Full error').element() as HTMLElement
			)?.state.doc.toString()
		).toBe(JSON.stringify(details, null, 2));
	});
});
