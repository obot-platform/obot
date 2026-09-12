import McpResult from './McpResult.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

function renderScrollableResult(height = 460) {
	const content = Array.from({ length: 20 }, (_, index) => ({
		type: 'text',
		text: `Response item ${index + 1}`
	}));
	render(McpResult, {
		result: { status: 'success', durationMs: 10, value: { content } },
		content
	});
	// Model the tester's independently scrolling details pane.
	const pane = page.getByRole('region', { name: 'Operation result' }).element() as HTMLElement;
	pane.style.cssText = `height:${height}px;overflow:auto;`;
	pane.scrollTop = pane.scrollHeight;
	return pane;
}

describe('McpResult', () => {
	it.each([460, 240])('scrolls the expanded raw response into a %i px pane', async (height) => {
		const pane = renderScrollableResult(height);
		const raw = page.getByLabelText('Raw MCP response');
		await expect.element(raw).not.toBeVisible();
		const before = pane.scrollTop;
		await page.getByText('Raw response', { exact: true }).click();
		await expect.element(raw).toBeVisible();
		await expect.poll(() => pane.scrollTop).toBeGreaterThan(before);
		await expect
			.poll(() => {
				const bounds = raw.element().getBoundingClientRect();
				const viewport = pane.getBoundingClientRect();
				return Math.min(bounds.bottom, viewport.bottom) - Math.max(bounds.top, viewport.top);
			})
			.toBeGreaterThan(Math.min(height - 40, 380));
	});

	it('also reveals the raw response when opened with the keyboard', async () => {
		const pane = renderScrollableResult();
		const before = pane.scrollTop;
		(page.getByText('Raw response', { exact: true }).element() as HTMLElement).focus();
		await userEvent.keyboard('{Enter}');
		await expect.element(page.getByLabelText('Raw MCP response')).toBeVisible();
		await expect.poll(() => pane.scrollTop).toBeGreaterThan(before);
	});
});
