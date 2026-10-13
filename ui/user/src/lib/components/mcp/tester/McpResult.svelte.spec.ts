import McpResult from './McpResult.svelte';
import { EditorView } from '@codemirror/view';
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
	it.each([
		{ status: 'cancelled' as const, title: 'Cancelled', icon: 'ban' },
		{ status: 'timeout' as const, title: 'Timed out', icon: 'clock-3' }
	])(
		'keeps $status results styled as warnings with full details available',
		async ({ status, title, icon }) => {
			render(McpResult, {
				result: {
					status,
					durationMs: 10,
					message: 'Operation stopped',
					details: { reason: status }
				}
			});
			await expect
				.element(page.getByRole('status').getByRole('heading', { name: title }))
				.toBeVisible();
			expect(page.getByRole('status').element().classList.contains('notification-alert')).toBe(
				true
			);
			await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
			await expect.element(page.getByCSS(`svg.lucide-${icon}`)).toBeVisible();
			await page.getByRole('button', { name: 'View full error' }).click();
			await expect.element(page.getByLabelText('Full error')).toBeVisible();
		}
	);

	it('previews the returned tool error rather than only its status', async () => {
		render(McpResult, {
			result: {
				status: 'mcp-error',
				durationMs: 10,
				value: { isError: true, content: [{ type: 'text', text: 'Rate limit exceeded' }] }
			}
		});
		await expect.element(page.getByRole('alert').getByText(/Rate limit exceeded/)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'View full error' })).toBeVisible();
	});

	it('shows protocol error details in the shared dialog', async () => {
		render(McpResult, {
			result: {
				status: 'transport-error',
				durationMs: 10,
				message: 'Invalid parameters',
				details: { code: -32602, data: { path: ['query'] } }
			}
		});
		await page.getByRole('button', { name: 'View full error' }).click();
		await expect.element(page.getByLabelText('Full error')).toBeVisible();
		const errorText = EditorView.findFromDOM(
			page.getByLabelText('Full error').element() as HTMLElement
		)?.state.doc.toString();
		expect(JSON.parse(errorText ?? '')).toEqual({ code: -32602, data: { path: ['query'] } });
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
