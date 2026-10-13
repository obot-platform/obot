import TesterError from './TesterError.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

describe('TesterError', () => {
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
