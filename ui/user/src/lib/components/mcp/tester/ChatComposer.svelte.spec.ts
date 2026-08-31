import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterChat } from '$lib/services/mcp/tester-chat.svelte';
import { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
import ChatComposer from './ChatComposer.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const server = {
	id: 'ms1composer',
	configured: true,
	deploymentStatus: 'Available',
	manifest: {
		name: 'Tester Server',
		runtime: 'remote',
		serverUserType: 'singleUser'
	}
} as unknown as MCPCatalogServer;

function mcpFetch() {
	return vi.fn<typeof fetch>(async (_input, init) => {
		if (init?.method === 'GET') return new Response(null, { status: 405 });
		const request = JSON.parse(String(init?.body)) as { id?: string | number; method: string };
		if (request.method === 'initialize') {
			return Response.json(
				{
					jsonrpc: '2.0',
					id: request.id,
					result: {
						protocolVersion: '2025-06-18',
						capabilities: {},
						serverInfo: { name: 'tester-server', version: '1.0.0' }
					}
				},
				{ headers: { 'Mcp-Session-Id': 'tester-session' } }
			);
		}
		return new Response(null, { status: 202 });
	});
}

async function renderComposer(chatFetch: typeof fetch = async () => new Response(null)) {
	const session = new MCPTesterSession(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		mcpFetch()
	);
	await session.initialize();
	const spy = vi.fn<typeof fetch>(chatFetch);
	const chat = new MCPTesterChat(session, server.id, spy);
	render(ChatComposer, { chat, session });
	const textarea = page.getByRole('textbox', { name: 'Message' });
	return { session, chat, spy, textarea };
}

function messageBox(): HTMLTextAreaElement {
	return document.getElementById('mcp-tester-chat-message') as HTMLTextAreaElement;
}

describe('ChatComposer', () => {
	it('starts one line tall, grows with the draft, then caps and scrolls', async () => {
		const { session, textarea } = await renderComposer();
		await expect.element(textarea).toBeVisible();
		const singleLine = messageBox().getBoundingClientRect().height;

		// A one-line draft must not overflow, or the box shows a scrollbar that
		// scrolls by a couple of pixels.
		expect(messageBox().scrollHeight).toBeLessThanOrEqual(messageBox().clientHeight);
		expect(messageBox().style.overflowY).toBe('hidden');

		await textarea.fill('one\ntwo\nthree');
		const grown = messageBox().getBoundingClientRect().height;
		expect(grown).toBeGreaterThan(singleLine);
		expect(grown).toBeLessThanOrEqual(128);
		expect(messageBox().style.overflowY).toBe('hidden');

		await textarea.fill(Array.from({ length: 20 }, (_, index) => `line ${index}`).join('\n'));
		expect(messageBox().style.height).toBe('128px');
		expect(messageBox().style.overflowY).toBe('auto');
		expect(messageBox().scrollHeight).toBeGreaterThan(messageBox().clientHeight);

		session.close();
	});

	it('swaps Send for Stop while the turn runs, keeping focus but blocking a second submit', async () => {
		let release!: () => void;
		const pending = new Promise<void>((resolve) => (release = resolve));
		const { session, chat, spy, textarea } = await renderComposer(async () => {
			await pending;
			return new Response('data: {"type":"completion","reason":"stop"}\n\n', {
				headers: { 'Content-Type': 'text/event-stream' }
			});
		});

		await textarea.fill('first');
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(textarea).toHaveValue('');
		await expect.element(textarea).not.toBeDisabled();
		expect(document.activeElement).toBe(messageBox());

		// The same control now stops the turn instead of offering another send.
		const stop = page.getByRole('button', { name: 'Stop' });
		await expect.element(stop).toBeVisible();
		await expect.element(stop).not.toBeDisabled();
		expect(page.getByRole('button', { name: 'Send' }).elements()).toHaveLength(0);

		// The user can keep typing, but Enter must not start a second turn.
		await textarea.fill('typed while streaming');
		messageBox().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
		expect(spy).toHaveBeenCalledTimes(1);
		await expect.element(textarea).toHaveValue('typed while streaming');

		await stop.click();
		await vi.waitFor(() => expect(chat.status).toBe('cancelled'));

		release();
		await vi.waitFor(() => expect(chat.canSend).toBe(true));
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeVisible();
		session.close();
	});
});
