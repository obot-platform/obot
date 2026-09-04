import type { MCPCatalogServer } from '$lib/services';
import {
	MCPTesterChat,
	MCP_TESTER_MAX_MODEL_BYTES,
	MCP_TESTER_MAX_ROUNDS
} from './tester-chat.svelte';
import { MCPTesterSession } from './tester.svelte';
import type { GetPromptResult, ReadResourceResult } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it, vi } from 'vitest';

const server = {
	id: 'ms1tester',
	configured: true,
	deploymentStatus: 'Available',
	manifest: {
		name: 'Tester Server',
		runtime: 'remote',
		serverUserType: 'singleUser'
	}
} as unknown as MCPCatalogServer;

interface MCPRequest {
	id?: string | number;
	method: string;
	params?: Record<string, unknown>;
}

function mcpFetch(
	capabilities: Record<string, unknown> = {},
	handleRequest?: (request: MCPRequest) => unknown
) {
	return vi.fn<typeof fetch>(async (_input, init) => {
		if (init?.method === 'GET') return new Response(null, { status: 405 });
		const request = JSON.parse(String(init?.body)) as MCPRequest;
		if (request.method === 'initialize') {
			return Response.json(
				{
					jsonrpc: '2.0',
					id: request.id,
					result: {
						protocolVersion: request.params?.protocolVersion,
						capabilities,
						serverInfo: { name: 'tester-server', version: '1.0.0' }
					}
				},
				{ headers: { 'Mcp-Session-Id': 'tester-session' } }
			);
		}
		if (request.method === 'notifications/initialized') return new Response(null, { status: 202 });
		const result = handleRequest?.(request);
		if (result !== undefined) {
			return Response.json({ jsonrpc: '2.0', id: request.id, result });
		}
		return new Response('unexpected MCP request', { status: 500 });
	});
}

function stream(...events: unknown[]): Response {
	return new Response(events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join(''), {
		headers: { 'Content-Type': 'text/event-stream' }
	});
}

async function readySession(
	capabilities: Record<string, unknown> = {},
	handleRequest?: (request: MCPRequest) => unknown
): Promise<MCPTesterSession> {
	const session = new MCPTesterSession(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		mcpFetch(capabilities, handleRequest)
	);
	await session.initialize();
	return session;
}

describe('MCPTesterChat', () => {
	it('snapshots tools on first send and streams normalized assistant text', async () => {
		let listCalls = 0;
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				listCalls += 1;
				return {
					tools: [
						{
							name: 'lookup',
							description: 'Look up a record',
							inputSchema: { type: 'object' }
						}
					]
				};
			}
		});
		const requests: Array<Record<string, unknown>> = [];
		const chatFetch = vi.fn<typeof fetch>(async (_input, init) => {
			requests.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
			return stream(
				{ type: 'assistant_message_start' },
				{ type: 'text_delta', delta: 'Hello ' },
				{ type: 'text_delta', delta: '**tester**' },
				{ type: 'completion', reason: 'stop' }
			);
		});
		const chat = new MCPTesterChat(session, server.id, chatFetch);

		await chat.send('Hello');
		await chat.send('Again');

		expect(listCalls).toBe(1);
		expect(chat.toolSnapshot?.map((tool) => tool.name)).toEqual(['lookup']);
		expect(requests[0]?.tools).toEqual([
			{
				name: 'lookup',
				description: 'Look up a record',
				inputSchema: { type: 'object' }
			}
		]);
		expect(chat.timeline.filter((message) => message.role === 'assistant')).toHaveLength(2);
		expect(chat.timeline[1]?.text).toBe('Hello **tester**');
		expect(session.activeWorkflow).toBeUndefined();
		session.close();
	});

	it('preserves staged roles and resources before final typed user text', async () => {
		const session = await readySession();
		session.stagePrompt('ordered prompt', {
			messages: [
				{ role: 'user', content: { type: 'text', text: 'prompt user' } },
				{ role: 'assistant', content: { type: 'text', text: 'prompt assistant' } }
			]
		} as GetPromptResult);
		session.stageResource('guide', {
			contents: [{ uri: 'file:///guide.txt', mimeType: 'text/plain', text: 'guide body' }]
		} as ReadResourceResult);
		let request: { messages?: unknown[] } = {};
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async (_input, init) => {
				request = JSON.parse(String(init?.body)) as { messages?: unknown[] };
				return stream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'done' },
					{ type: 'completion', reason: 'stop' }
				);
			})
		);

		await chat.send('final question');

		expect(request.messages).toMatchObject([
			{ role: 'user', content: [{ type: 'text', text: 'prompt user' }] },
			{ role: 'assistant', content: [{ type: 'text', text: 'prompt assistant' }] },
			{
				role: 'user',
				content: [
					{
						type: 'resource',
						text: 'guide body',
						uri: 'file:///guide.txt',
						mimeType: 'text/plain'
					}
				]
			},
			{ role: 'user', content: [{ type: 'text', text: 'final question' }] }
		]);
		expect(session.stagedContext).toEqual([]);
		session.close();
	});

	it('collects individual decisions, runs approvals in order, and continues with all results', async () => {
		const calledTools: string[] = [];
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return {
					tools: [
						{ name: 'first', inputSchema: { type: 'object' } },
						{ name: 'second', inputSchema: { type: 'object' } },
						{ name: 'third', inputSchema: { type: 'object' } }
					]
				};
			}
			if (request.method === 'tools/call') {
				calledTools.push(String(request.params?.name));
				return request.params?.name === 'first'
					? {
							content: [{ type: 'text', text: 'declared failure' }],
							isError: true
						}
					: { content: [{ type: 'text', text: 'success' }], isError: false };
			}
		});
		const modelRequests: Array<{ messages: Array<Record<string, unknown>> }> = [];
		const chatFetch = vi.fn<typeof fetch>(async (_input, init) => {
			modelRequests.push(
				JSON.parse(String(init?.body)) as { messages: Array<Record<string, unknown>> }
			);
			if (modelRequests.length === 1) {
				return stream(
					{ type: 'assistant_message_start' },
					{
						type: 'tool_calls',
						calls: [
							{ id: 'call-1', name: 'first', arguments: { order: 1 } },
							{ id: 'call-2', name: 'second', arguments: { order: 2 } },
							{ id: 'call-3', name: 'third', arguments: { order: 3 } }
						]
					},
					{ type: 'completion', reason: 'tool_calls' }
				);
			}
			return stream(
				{ type: 'assistant_message_start' },
				{ type: 'text_delta', delta: 'I handled both results.' },
				{ type: 'completion', reason: 'stop' }
			);
		});
		const chat = new MCPTesterChat(session, server.id, chatFetch);

		await chat.send('Use both');
		expect(chat.status).toBe('approval');
		expect(() => session.beginWorkflow('direct', 'direct call')).toThrow(/chat turn is active/);

		chat.reject('call-3');
		expect(calledTools).toEqual([]);
		chat.approve('call-2');
		expect(calledTools).toEqual([]);
		chat.approve('call-1');
		await vi.waitFor(() => expect(chat.status).toBe('idle'));

		expect(calledTools).toEqual(['first', 'second']);
		expect(modelRequests).toHaveLength(2);
		expect(modelRequests[1]?.messages.slice(-3)).toMatchObject([
			{ role: 'tool', toolResult: { callID: 'call-1', status: 'error' } },
			{ role: 'tool', toolResult: { callID: 'call-2', status: 'success' } },
			{
				role: 'tool',
				toolResult: {
					callID: 'call-3',
					status: 'rejected',
					content: 'rejected by user'
				}
			}
		]);
		expect(chat.timeline.at(-1)?.text).toBe('I handled both results.');
		session.close();
	});

	it('returns cancellation to the model when an executing approved tool is cancelled', async () => {
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return { tools: [{ name: 'slow', inputSchema: { type: 'object' } }] };
			}
		});
		vi.spyOn(session, 'callChatTool').mockImplementation(
			async (_name, _args, signal) =>
				await new Promise((resolve) => {
					signal.addEventListener(
						'abort',
						() =>
							resolve({
								status: 'cancelled',
								durationMs: 1,
								message: 'Cancelled by user'
							}),
						{ once: true }
					);
				})
		);
		const requests: Array<{ messages: Array<Record<string, unknown>> }> = [];
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async (_input, init) => {
				requests.push(
					JSON.parse(String(init?.body)) as { messages: Array<Record<string, unknown>> }
				);
				return requests.length === 1
					? stream(
							{ type: 'assistant_message_start' },
							{
								type: 'tool_calls',
								calls: [{ id: 'call-slow', name: 'slow', arguments: {} }]
							},
							{ type: 'completion', reason: 'tool_calls' }
						)
					: stream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'The slow call was cancelled.' },
							{ type: 'completion', reason: 'stop' }
						);
			})
		);

		await chat.send('Run slowly');
		chat.approve('call-slow');
		await vi.waitFor(() => expect(chat.status).toBe('executing-tools'));
		chat.cancelExecutingTool();
		await vi.waitFor(() => expect(chat.status).toBe('idle'));

		expect(requests[1]?.messages.at(-1)).toMatchObject({
			role: 'tool',
			toolResult: {
				callID: 'call-slow',
				status: 'cancelled',
				content: { category: 'cancelled', message: 'Cancelled by user' }
			}
		});
		expect(chat.timeline.at(-1)?.text).toBe('The slow call was cancelled.');
		session.close();
	});

	it('retries a failed continuation without executing a completed tool again', async () => {
		let executions = 0;
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return { tools: [{ name: 'once', inputSchema: { type: 'object' } }] };
			}
			if (request.method === 'tools/call') {
				executions += 1;
				return { content: [{ type: 'text', text: 'completed once' }], isError: false };
			}
		});
		let modelCalls = 0;
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async () => {
				modelCalls += 1;
				if (modelCalls === 1) {
					return stream(
						{ type: 'assistant_message_start' },
						{
							type: 'tool_calls',
							calls: [{ id: 'call-once', name: 'once', arguments: {} }]
						},
						{ type: 'completion', reason: 'tool_calls' }
					);
				}
				if (modelCalls === 2) {
					return stream(
						{ type: 'assistant_message_start' },
						{ type: 'text_delta', delta: 'partial continuation' },
						{
							type: 'error',
							error: { code: 'provider_error', message: 'retry continuation', retryable: true }
						}
					);
				}
				return stream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'completed continuation' },
					{ type: 'completion', reason: 'stop' }
				);
			})
		);

		await chat.send('Call once');
		chat.approve('call-once');
		await vi.waitFor(() => expect(chat.status).toBe('failed'));
		expect(executions).toBe(1);

		await chat.retry();
		expect(executions).toBe(1);
		expect(modelCalls).toBe(3);
		expect(chat.timeline.at(-1)?.text).toBe('completed continuation');
		session.close();
	});

	it('keeps a partial failed attempt and replaces it on retry', async () => {
		const session = await readySession();
		let calls = 0;
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async () => {
				calls += 1;
				return calls === 1
					? stream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'partial answer' },
							{
								type: 'error',
								error: { code: 'provider_error', message: 'provider failed', retryable: true }
							}
						)
					: stream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'replacement answer' },
							{ type: 'completion', reason: 'stop' }
						);
			})
		);

		await chat.send('Try');
		const attemptID = chat.timeline.at(-1)?.id;
		expect(chat.timeline.at(-1)).toMatchObject({
			text: 'partial answer',
			state: 'failed'
		});

		await chat.retry();
		expect(chat.timeline.filter((message) => message.role === 'assistant')).toHaveLength(1);
		expect(chat.timeline.at(-1)).toMatchObject({
			id: attemptID,
			text: 'replacement answer',
			state: 'complete'
		});
		expect(calls).toBe(2);
		session.close();
	});

	it('keeps an oversized tool result visible but sends a small synthetic result to the model', async () => {
		const oversized = 'x'.repeat(MCP_TESTER_MAX_MODEL_BYTES + 1);
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return { tools: [{ name: 'large', inputSchema: { type: 'object' } }] };
			}
			if (request.method === 'tools/call') {
				return { content: [{ type: 'text', text: oversized }], isError: false };
			}
		});
		const requests: Array<{ messages: Array<Record<string, unknown>> }> = [];
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async (_input, init) => {
				requests.push(
					JSON.parse(String(init?.body)) as { messages: Array<Record<string, unknown>> }
				);
				return requests.length === 1
					? stream(
							{ type: 'assistant_message_start' },
							{
								type: 'tool_calls',
								calls: [{ id: 'call-large', name: 'large', arguments: {} }]
							},
							{ type: 'completion', reason: 'tool_calls' }
						)
					: stream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'Large result noted.' },
							{ type: 'completion', reason: 'stop' }
						);
			})
		);

		await chat.send('Get the large result');
		chat.approve('call-large');
		await vi.waitFor(() => expect(chat.status).toBe('idle'));

		const visibleResult = chat.timeline
			.flatMap((message) => message.toolCalls ?? [])
			.find((call) => call.id === 'call-large')?.result;
		expect(visibleResult?.value?.content).toMatchObject([{ text: oversized }]);
		expect(requests[1]?.messages.at(-1)).toMatchObject({
			toolResult: {
				status: 'success',
				content: {
					oversized: true,
					message: 'The tool succeeded, but its output was too large to include.'
				}
			}
		});
		session.close();
	});

	it('enforces the round cap after sequentially resolving the tenth batch', async () => {
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return { tools: [{ name: 'loop', inputSchema: { type: 'object' } }] };
			}
			if (request.method === 'tools/call') {
				return { content: [{ type: 'text', text: 'ok' }] };
			}
		});
		let modelRound = 0;
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async () => {
				modelRound += 1;
				return stream(
					{ type: 'assistant_message_start' },
					{
						type: 'tool_calls',
						calls: [{ id: `call-${modelRound}`, name: 'loop', arguments: {} }]
					},
					{ type: 'completion', reason: 'tool_calls' }
				);
			})
		);

		await chat.send('Loop');
		for (let round = 1; round <= MCP_TESTER_MAX_ROUNDS; round += 1) {
			expect(chat.status).toBe('approval');
			chat.approve(`call-${round}`);
			await vi.waitFor(() =>
				expect(chat.status).toBe(round === MCP_TESTER_MAX_ROUNDS ? 'round-limit' : 'approval')
			);
		}

		expect(modelRound).toBe(MCP_TESTER_MAX_ROUNDS);
		expect(chat.error?.message).toContain('Send Continue');
		expect(session.activeWorkflow).toBeUndefined();
		session.close();
	});

	it('cancels active generation and clears all page-memory chat state on New Chat', async () => {
		const session = await readySession();
		const chatFetch = vi.fn<typeof fetch>(
			async (_input, init) =>
				await new Promise<Response>((_resolve, reject) => {
					init?.signal?.addEventListener(
						'abort',
						() => reject(new DOMException('Aborted', 'AbortError')),
						{ once: true }
					);
				})
		);
		const chat = new MCPTesterChat(session, server.id, chatFetch);

		void chat.send('Wait');
		await vi.waitFor(() => expect(chat.status).toBe('streaming'));
		chat.stop();
		expect(chat.status).toBe('cancelled');
		expect(session.activeWorkflow).toBeUndefined();

		chat.newChat();
		expect(chat.messages).toEqual([]);
		expect(chat.timeline).toEqual([]);
		expect(chat.toolSnapshot).toBeUndefined();
		expect(chat.error).toBeUndefined();
		session.close();
	});

	it('drops a tool result that lands after New Chat cleared the conversation', async () => {
		const session = await readySession({ tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return { tools: [{ name: 'slow', inputSchema: { type: 'object' } }] };
			}
		});
		let toolSettled = false;
		vi.spyOn(session, 'callChatTool').mockImplementation(
			async (_name, _args, signal) =>
				await new Promise((resolve) => {
					signal.addEventListener(
						'abort',
						() => {
							toolSettled = true;
							resolve({ status: 'cancelled', durationMs: 1, message: 'Cancelled by user' });
						},
						{ once: true }
					);
				})
		);
		const chat = new MCPTesterChat(
			session,
			server.id,
			vi.fn<typeof fetch>(async () =>
				stream(
					{ type: 'assistant_message_start' },
					{ type: 'tool_calls', calls: [{ id: 'call-slow', name: 'slow', arguments: {} }] },
					{ type: 'completion', reason: 'tool_calls' }
				)
			)
		);

		await chat.send('Run slowly');
		chat.approve('call-slow');
		await vi.waitFor(() => expect(chat.status).toBe('executing-tools'));

		chat.newChat();
		expect(session.activeWorkflow).toBeUndefined();

		// The in-flight call resolves against the aborted signal after New Chat, and its
		// result answers a tool call the fresh conversation no longer contains.
		await vi.waitFor(() => expect(toolSettled).toBe(true));
		await vi.waitFor(() => expect(chat.status).toBe('idle'));
		expect(chat.messages).toEqual([]);
		session.close();
	});
});
