import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterChat } from '$lib/services/mcp/tester-chat.svelte';
import { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
import Chat from './Chat.svelte';
import type { CallToolResult, Tool } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const server = {
	id: 'ms1chat',
	configured: true,
	deploymentStatus: 'Available',
	manifest: {
		name: 'Tester Server',
		runtime: 'remote',
		serverUserType: 'singleUser'
	}
} as unknown as MCPCatalogServer;

const RESPONSE =
	'| Tool | Description |\n| --- | --- |\n| `add` | Add two numbers |\n\n---\n\nDone.';

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

async function renderChatWithResponse() {
	const session = new MCPTesterSession(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		mcpFetch()
	);
	await session.initialize();
	const chat = new MCPTesterChat(
		session,
		server.id,
		vi.fn<typeof fetch>(
			async () =>
				new Response(
					[
						'data: {"type":"assistant_message_start"}\n\n',
						`data: ${JSON.stringify({ type: 'text_delta', delta: RESPONSE })}\n\n`,
						'data: {"type":"completion","reason":"stop"}\n\n'
					].join(''),
					{ headers: { 'Content-Type': 'text/event-stream' } }
				)
		)
	);
	await chat.send('list the tools');
	render(Chat, { chat, session });
	return session;
}

function sse(...events: Array<Record<string, unknown>>): Response {
	return new Response(events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join(''), {
		headers: { 'Content-Type': 'text/event-stream' }
	});
}

async function renderChatWithToolCalls() {
	const session = new MCPTesterSession(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		mcpFetch()
	);
	await session.initialize();
	vi.spyOn(session, 'snapshotChatTools').mockResolvedValue([
		{ name: 'add', inputSchema: { type: 'object' } },
		{ name: 'subtract', inputSchema: { type: 'object' } }
	] as Tool[]);
	const callTool = vi.spyOn(session, 'callChatTool').mockResolvedValue({
		status: 'success',
		durationMs: 1,
		value: { content: [{ type: 'text', text: 'tool output' }] } as CallToolResult
	});

	let turn = 0;
	const chat = new MCPTesterChat(
		session,
		server.id,
		vi.fn<typeof fetch>(async () => {
			turn += 1;
			if (turn === 1) {
				return sse(
					{ type: 'assistant_message_start' },
					{
						type: 'tool_calls',
						calls: [
							{ id: 'call-add', name: 'add', arguments: { a: 1 } },
							{ id: 'call-subtract', name: 'subtract', arguments: { b: 2 } }
						]
					},
					{ type: 'completion', reason: 'tool_calls' }
				);
			}
			return sse(
				{ type: 'assistant_message_start' },
				{ type: 'text_delta', delta: 'All done.' },
				{ type: 'completion', reason: 'stop' }
			);
		})
	);
	render(Chat, { chat, session });
	await chat.send('add then subtract');
	return { session, chat, callTool };
}

function approvalPrompt() {
	return page.getByRole('region', { name: 'Tool approval' });
}

function transcriptCalls() {
	return page.getByRole('region', { name: 'Tool calls' });
}

describe('Chat', () => {
	it('renders the assistant response in its own bordered card with markdown styling', async () => {
		const session = await renderChatWithResponse();
		const article = page.getByRole('article', { name: 'Assistant message' });
		await expect.element(article).toBeVisible();

		const card = article.element().querySelector('.milkdown-content')?.parentElement;
		expect(card?.className).toContain('rounded-lg');
		expect(card?.className).toContain('border');
		expect(article.element().querySelector('.tester-markdown')).not.toBeNull();
		await expect.element(page.getByRole('cell', { name: 'Add two numbers' })).toBeVisible();

		session.close();
	});

	it('exposes an icon-only copy control for the response', async () => {
		const session = await renderChatWithResponse();
		const copy = page.getByRole('button', { name: 'Copy response' });

		await expect.element(copy).toBeInTheDocument();
		expect(copy.element().textContent?.trim()).toBe('');

		session.close();
	});
	it('prompts for one tool at a time, above the composer rather than in the transcript', async () => {
		const { session } = await renderChatWithToolCalls();

		await expect.element(approvalPrompt()).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Approve add' })).toBeVisible();
		await expect.element(page.getByText('Tool request 1 of 2')).toBeVisible();

		// The second request stays queued until the first one is decided, and
		// nothing reaches the transcript before a call has run.
		expect(page.getByRole('button', { name: 'Approve subtract' }).elements()).toHaveLength(0);
		expect(transcriptCalls().elements()).toHaveLength(0);

		// The prompt sits between the transcript and the composer.
		const composer = page.getByRole('textbox', { name: 'Message' }).element();
		expect(
			approvalPrompt().element().compareDocumentPosition(composer) &
				Node.DOCUMENT_POSITION_FOLLOWING
		).toBeTruthy();

		session.close();
	});

	it('moves each tool into the transcript once it finishes and advances the queue', async () => {
		const { session, chat, callTool } = await renderChatWithToolCalls();
		await page.getByRole('button', { name: 'Approve add' }).click();

		await expect
			.element(transcriptCalls().getByRole('article', { name: 'add tool call' }))
			.toBeVisible();
		await expect.element(page.getByText('Tool request 2 of 2')).toBeVisible();
		expect(callTool).toHaveBeenCalledTimes(1);

		await page.getByRole('button', { name: 'Reject subtract' }).click();

		await expect
			.element(transcriptCalls().getByRole('article', { name: 'subtract tool call' }))
			.toBeVisible();
		await expect.element(approvalPrompt()).not.toBeInTheDocument();
		await vi.waitFor(() => expect(chat.status).toBe('idle'));
		// A rejected call is recorded but never executed.
		expect(callTool).toHaveBeenCalledTimes(1);

		session.close();
	});
});
