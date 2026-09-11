import McpContent from './McpContent.svelte';
import McpTextResult from './McpTextResult.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const longText = Array.from({ length: 100 }, (_, i) => `Response line ${i + 1}`).join('\n');

describe('McpTextResult', () => {
	it('keeps all controls and fullscreen content within a narrow viewport', async () => {
		await page.viewport(390, 844);
		try {
			render(McpTextResult, { text: JSON.stringify({ lines: longText.split('\n') }) });
			const result = page.getByRole('region', { name: 'Text result' });
			await expect.element(result).toBeVisible();
			for (const button of result.getByRole('button').all()) {
				const bounds = button.element().getBoundingClientRect();
				expect(bounds.left).toBeGreaterThanOrEqual(0);
				expect(bounds.right).toBeLessThanOrEqual(390);
			}
			await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
			const full = page.getByLabelText('Full text', { exact: true });
			await expect.element(full).toBeVisible();
			const bounds = full.element().getBoundingClientRect();
			expect(bounds.height).toBeGreaterThan(500);
			expect(bounds.bottom).toBeLessThanOrEqual(844);
			await page.getByRole('button', { name: 'Close fullscreen' }).click();
		} finally {
			await page.viewport(1280, 720);
		}
	});

	it('preserves numeric IDs, duplicate keys, escapes, and empty containers when formatting', async () => {
		const text =
			'{"id":9007199254740993,"value":1e400,"key":1,"key":2,"escaped":"\\u0061\\"[,]","empty":[{},[]]}';
		render(McpTextResult, { text });
		const full = page.getByLabelText('Full text', { exact: true });
		await expect.element(full).toHaveTextContent('9007199254740993');
		await expect.element(full).toHaveTextContent('1e400');
		await expect.element(full).toHaveTextContent('"key": 1, "key": 2');
		await expect.element(full).toHaveTextContent('"escaped": "\\u0061\\"[,]"');
		await expect.element(full).toHaveTextContent('"empty": [ {}, [] ]');
	});

	it('formats valid JSON and can show its original whitespace', async () => {
		const text = '{ "folders" : ["Inbox", "Archive"] }';
		render(McpTextResult, { text });
		const full = page.getByLabelText('Full text', { exact: true });
		await expect.element(full).toBeVisible();
		expect(full.element().textContent).toBe(JSON.stringify(JSON.parse(text), null, 2));
		await expect
			.element(page.getByRole('button', { name: 'Show full text' }))
			.not.toBeInTheDocument();
		const format = page.getByRole('button', { name: 'Format JSON' });
		await expect.element(format).toHaveAttribute('aria-pressed', 'true');
		await format.click();
		await expect.element(format).toHaveAttribute('aria-pressed', 'false');
		expect(full.element().textContent).toBe(text);
	});

	it('keeps invalid JSON and server markup as literal text', async () => {
		const text = '{invalid JSON}\n<script>window.compromised = true</script>';
		render(McpTextResult, { text });
		await expect
			.element(page.getByLabelText('Full text', { exact: true }))
			.toHaveTextContent(text, { normalizeWhitespace: false });
		await expect.element(page.getByRole('button', { name: 'Format JSON' })).not.toBeInTheDocument();
		await expect.element(page.getByCSS('pre script')).not.toBeInTheDocument();
	});

	it('copies the complete original text from preview, expanded, and fullscreen states', async () => {
		const text = JSON.stringify({ lines: longText.split('\n') });
		const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
		try {
			render(McpTextResult, { text });
			await page.getByRole('button', { name: 'Copy full text', exact: true }).click();
			expect(copy).toHaveBeenLastCalledWith(text);
			await page.getByRole('button', { name: 'Show full text', exact: true }).click();
			await page.getByRole('button', { name: /Copy full text|Copied!/ }).click();
			expect(copy).toHaveBeenLastCalledWith(text);
			await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
			await page.getByRole('button', { name: 'Copy full text', exact: true }).click();
			expect(copy).toHaveBeenCalledTimes(3);
			expect(copy).toHaveBeenLastCalledWith(text);
		} finally {
			copy.mockRestore();
		}
	});

	it('keeps the toolbar outside the scroll area and restores position and focus after Escape', async () => {
		render(McpTextResult, { text: longText });
		await page.getByRole('button', { name: 'Show full text', exact: true }).click();
		const full = page.getByLabelText('Full text', { exact: true });
		const pre = full.element() as HTMLElement;
		pre.scrollTop = 200;
		expect(pre.scrollTop).toBe(200);
		const copy = page.getByRole('button', { name: 'Copy full text', exact: true });
		expect(copy.element().getBoundingClientRect().bottom).toBeLessThanOrEqual(
			pre.getBoundingClientRect().top
		);
		await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(page.getByLabelText('Text preview')).not.toBeInTheDocument();
		expect(page.getByCSS('pre').all()).toHaveLength(1);
		await expect.element(page.getByRole('button', { name: 'Close fullscreen' })).toHaveFocus();
		(full.element() as HTMLElement).scrollTop = 500;
		await userEvent.keyboard('{Escape}');
		await expect.element(page.getByRole('dialog', { includeHidden: true })).not.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Fullscreen', exact: true }))
			.toHaveFocus();
		await expect.element(page.getByRole('button', { name: 'Show less' })).toBeVisible();
		expect((full.element() as HTMLElement).scrollTop).toBe(200);
	});

	it('returns to the preview when fullscreen was opened from a collapsed result', async () => {
		render(McpTextResult, { text: longText });
		await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
		await expect
			.element(page.getByLabelText('Full text', { exact: true }))
			.toHaveTextContent('Response line 100');
		await page.getByRole('button', { name: 'Close fullscreen' }).click();
		await expect.element(page.getByLabelText('Text preview')).toBeVisible();
		await expect.element(page.getByLabelText('Full text', { exact: true })).not.toBeInTheDocument();
	});

	it('resets expansion when a tool returns new text', async () => {
		const props = $state({ content: { type: 'text', text: longText }, collapseLongText: true });
		render(McpContent, props);
		await page.getByRole('button', { name: 'Show full text', exact: true }).click();
		props.content = { type: 'text', text: `New result\n${longText}` };
		await expect.element(page.getByLabelText('Text preview')).toHaveTextContent('New result');
		await expect.element(page.getByLabelText('Full text', { exact: true })).not.toBeInTheDocument();
	});

	it('restores page scrolling when a fullscreen result is replaced', async () => {
		const previousOverflow = document.body.style.overflow;
		const props = $state({ content: { type: 'text', text: longText }, collapseLongText: true });
		render(McpContent, props);
		await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
		await expect.element(page.getByRole('dialog')).toBeVisible();
		expect(document.body.style.overflow).toBe('hidden');
		props.content = { type: 'text', text: `Replacement\n${longText}` };
		await expect.element(page.getByLabelText('Text preview')).toHaveTextContent('Replacement');
		expect(document.body.style.overflow).toBe(previousOverflow);
		await expect.element(page.getByRole('dialog', { includeHidden: true })).not.toBeVisible();
	});
});
