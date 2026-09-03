import type { TesterToolApproval } from '$lib/services/mcp/tester-chat.svelte';
import ToolCallRecord from './ToolCallRecord.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function makeCall(overrides: Partial<TesterToolApproval> = {}): TesterToolApproval {
	return {
		id: 'call_1',
		name: 'echo',
		arguments: { message: 'howdy!' },
		status: 'approved',
		modelResult: { status: 'success', durationMs: 151, value: { content: [] } },
		...overrides
	} as TesterToolApproval;
}

describe('ToolCallRecord', () => {
	it('anchors the arguments copy control inside the arguments box', async () => {
		render(ToolCallRecord, { calls: [makeCall()] });

		await page.getByRole('group').getByText('Arguments').click();

		const pre = page.getByLabelText('echo arguments').element();
		const copy = page.getByRole('button', { name: 'Copy arguments' });
		await expect.element(copy).toBeVisible();

		const button = copy.element().getBoundingClientRect();
		const box = pre.getBoundingClientRect();
		const reserved = Number.parseFloat(getComputedStyle(pre).paddingRight);

		// The control belongs in the box's top-right corner, not on a row above it.
		expect(button.top).toBeGreaterThanOrEqual(box.top - 1);
		expect(button.bottom).toBeLessThanOrEqual(box.bottom + 1);
		expect(box.right - button.right).toBeLessThanOrEqual(12);
		// Reserved padding keeps it clear of the JSON text.
		expect(reserved).toBeGreaterThan(0);
		expect(button.left).toBeGreaterThanOrEqual(box.right - reserved - 1);
	});
});
