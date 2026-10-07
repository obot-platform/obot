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
	it('also reveals the raw response when opened with the keyboard', async () => {
		const pane = renderScrollableResult();
		const before = pane.scrollTop;
		(page.getByText('Raw response', { exact: true }).element() as HTMLElement).focus();
		await userEvent.keyboard('{Enter}');
		await expect.element(page.getByLabelText('Raw MCP response')).toBeVisible();
		await expect.poll(() => pane.scrollTop).toBeGreaterThan(before);
	});
});
