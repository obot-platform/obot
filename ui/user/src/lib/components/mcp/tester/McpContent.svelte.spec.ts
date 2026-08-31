import McpContent from './McpContent.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('McpContent', () => {
	it('escapes text content instead of rendering server-provided markup', async () => {
		render(McpContent, {
			content: { type: 'text', text: '<script>window.compromised = true</script>' }
		});

		await expect
			.element(page.getByText('<script>window.compromised = true</script>', { exact: true }))
			.toBeVisible();
		await expect.element(page.getByCSS('pre script')).not.toBeInTheDocument();
	});

	it('keeps the corner copy button inside short single-line content', async () => {
		render(McpContent, { content: { type: 'text', text: '{"sum":100}' } });

		const copy = page.getByRole('button', { name: 'Copy text' });
		await expect.element(copy).toBeVisible();

		const button = copy.element().getBoundingClientRect();
		const box = (copy.element().parentElement as HTMLElement).getBoundingClientRect();
		const pre = page.getByLabelText('Text content').element();
		const text = pre.getBoundingClientRect();
		const reserved = Number.parseFloat(getComputedStyle(pre).paddingRight);

		// A single line is shorter than the button, so anchoring the button to the text wrapper
		// leaves it hanging below the line. It must sit in the box's top-right corner, starting
		// no lower than the first line of text.
		expect(button.top).toBeLessThanOrEqual(text.top + 4);
		expect(button.bottom).toBeLessThanOrEqual(box.bottom + 1);
		expect(box.right - button.right).toBeLessThanOrEqual(12);
		// ...and the text's reserved right padding must keep it clear of the content.
		expect(reserved).toBeGreaterThan(0);
		expect(button.left).toBeGreaterThanOrEqual(text.right - reserved - 1);
	});

	it('previews long tool-result text and reveals the complete content on demand', async () => {
		const text = `Preview starts here. ${'x'.repeat(2100)} Full content ends here.`;
		render(McpContent, {
			content: { type: 'text', text },
			collapseLongText: true
		});

		const preview = page.getByLabelText('Text preview');
		const fullText = page.getByLabelText('Full text');
		await expect.element(preview).toBeVisible();
		await expect.element(preview).toHaveTextContent('Preview starts here.');
		await expect.element(preview).not.toHaveTextContent('Full content ends here.');
		await expect.element(fullText).not.toBeVisible();

		await page.getByText('Show full text', { exact: true }).click();
		await expect.element(fullText).toBeVisible();
		await expect.element(fullText).toHaveTextContent('Full content ends here.');
	});

	it('leaves long text expanded outside tool results', async () => {
		const text = `Always visible. ${'x'.repeat(2100)} Expanded content ends here.`;
		render(McpContent, { content: { type: 'text', text } });

		await expect.element(page.getByLabelText('Text content')).toBeVisible();
		await expect
			.element(page.getByLabelText('Text content'))
			.toHaveTextContent('Expanded content ends here.');
		await expect.element(page.getByText('Show full text', { exact: true })).not.toBeInTheDocument();
	});

	it('links only safe external HTTP resource URLs', async () => {
		const unsafe = await render(McpContent, {
			content: { type: 'resource_link', uri: 'javascript:alert(1)', name: 'Unsafe' }
		});
		await expect.element(page.getByText('Unsafe', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('link', { name: 'Unsafe' })).not.toBeInTheDocument();
		unsafe.unmount();

		render(McpContent, {
			content: { type: 'resource_link', uri: 'https://example.com/resource', name: 'Safe' }
		});
		const safe = page.getByRole('link', { name: 'Safe' });
		await expect.element(safe).toHaveAttribute('href', 'https://example.com/resource');
		await expect.element(safe).toHaveAttribute('rel', 'noopener noreferrer');
		await expect.element(safe).toHaveAttribute('target', '_blank');
	});
});
