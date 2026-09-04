import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterSession, normalizeTesterSection } from './tester.svelte';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
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

function readyFetch(
	capabilities: Record<string, unknown> = {},
	handleRequest?: (request: MCPRequest) => Response | undefined
) {
	return vi.fn<typeof fetch>(async (_input, init) => {
		if (init?.method === 'GET') {
			return new Response(null, { status: 405 });
		}

		const request = JSON.parse(String(init?.body)) as MCPRequest;
		const handled = handleRequest?.(request);
		if (handled) return handled;
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
		if (request.method === 'notifications/initialized') {
			return new Response(null, { status: 202 });
		}
		return new Response('unexpected MCP request', { status: 500 });
	});
}

describe('MCPTesterSession', () => {
	it('defaults missing and invalid tab values to chat', () => {
		expect(normalizeTesterSection(undefined)).toBe('chat');
		expect(normalizeTesterSection('invalid')).toBe('chat');
		expect(normalizeTesterSection('logs')).toBe('logs');
		expect(normalizeTesterSection('resources')).toBe('resources');
	});

	it('uses the official SDK with tester identity, no capabilities, and memory-only state', async () => {
		const fetcher = readyFetch();
		const localGet = vi.spyOn(Storage.prototype, 'getItem');
		const localSet = vi.spyOn(Storage.prototype, 'setItem');
		const localRemove = vi.spyOn(Storage.prototype, 'removeItem');
		const sessionGet = vi.spyOn(sessionStorage, 'getItem');
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: '1.2.3' },
			fetcher
		);

		expect(session.client).toBeInstanceOf(Client);
		await session.initialize();

		const initializeCall = fetcher.mock.calls.find(([, init]) => {
			if (typeof init?.body !== 'string') return false;
			return (JSON.parse(init.body) as MCPRequest).method === 'initialize';
		});
		expect(initializeCall).toBeDefined();
		const initialize = JSON.parse(String(initializeCall?.[1]?.body)) as MCPRequest;
		expect(initialize.params?.clientInfo).toEqual({
			name: 'obot-mcp-tester',
			version: '1.2.3'
		});
		expect(initialize.params?.capabilities).toEqual({});
		expect(new URL(String(initializeCall?.[0])).searchParams.get('method')).toBe('initialize');
		expect(initializeCall?.[1]?.credentials).toBe('same-origin');
		expect(localGet).not.toHaveBeenCalled();
		expect(localSet).not.toHaveBeenCalled();
		expect(localRemove).not.toHaveBeenCalled();
		expect(sessionGet).not.toHaveBeenCalled();

		const signal = initializeCall?.[1]?.signal;
		session.close();
		expect(signal?.aborted).toBe(true);
		expect(localRemove).not.toHaveBeenCalled();
	});

	it('uses typed SDK list calls to follow cursors and retain raw pages', async () => {
		let page = 0;
		const fetcher = readyFetch({ tools: {} }, (request) => {
			if (request.method !== 'tools/list') return;
			page += 1;
			return Response.json({
				jsonrpc: '2.0',
				id: request.id,
				result:
					page === 1
						? {
								tools: [{ name: 'first', inputSchema: { type: 'object' } }],
								nextCursor: 'next-page'
							}
						: { tools: [{ name: 'second', inputSchema: { type: 'object' } }] }
			});
		});
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			fetcher
		);
		await session.initialize();

		const result = await session.listTools();

		expect(result.items.map((tool) => tool.name)).toEqual(['first', 'second']);
		expect(result.pages).toHaveLength(2);
		const listRequests = fetcher.mock.calls
			.filter(([, init]) => {
				if (typeof init?.body !== 'string') return false;
				return (JSON.parse(init.body) as MCPRequest).method === 'tools/list';
			})
			.map(([, init]) => (JSON.parse(String(init?.body)) as MCPRequest).params);
		expect(listRequests).toEqual([{}, { cursor: 'next-page' }]);
		session.close();
	});

	it('loads advertised capabilities lazily, caches them, and refreshes on request', async () => {
		let listCalls = 0;
		const fetcher = readyFetch({ prompts: {} }, (request) => {
			if (request.method !== 'prompts/list') return;
			listCalls += 1;
			return Response.json({
				jsonrpc: '2.0',
				id: request.id,
				result: { prompts: [{ name: `prompt-${listCalls}` }] }
			});
		});
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			fetcher
		);
		await session.initialize();

		expect(listCalls).toBe(0);
		await session.loadPrompts();
		await session.loadPrompts();
		expect(listCalls).toBe(1);
		expect(session.cache.prompts.items[0]?.name).toBe('prompt-1');

		await session.loadPrompts(true);
		expect(listCalls).toBe(2);
		expect(session.cache.prompts.items[0]?.name).toBe('prompt-2');
		session.close();
	});

	it('marks unadvertised capabilities unsupported without issuing a list request', async () => {
		const fetcher = readyFetch();
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			fetcher
		);
		await session.initialize();
		await session.loadResources();

		expect(session.cache.resources.unsupported).toBe(true);
		expect(
			fetcher.mock.calls.some(([, init]) => String(init?.body).includes('resources/list'))
		).toBe(false);
		session.close();
	});

	it('stages only supported prompt and resource text within the 100 KiB limit', () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>()
		);
		const prompt = {
			messages: [
				{ role: 'user', content: { type: 'text', text: 'first' } },
				{ role: 'assistant', content: { type: 'text', text: 'second' } }
			]
		} as GetPromptResult;
		const resource = {
			contents: [{ uri: 'file:///guide.txt', mimeType: 'text/plain', text: 'guide' }]
		} as ReadResourceResult;

		expect(session.stagePrompt('ordered', prompt)).toEqual({ ok: true });
		expect(session.stageResource('guide', resource)).toEqual({ ok: true });
		expect(session.stagedContext).toHaveLength(2);
		expect(session.stagedContext[0]).toMatchObject({ type: 'prompt', messages: prompt.messages });
		expect(
			session.stageResource('binary', {
				contents: [{ uri: 'file:///image.png', mimeType: 'image/png', blob: 'AAAA' }]
			} as ReadResourceResult)
		).toMatchObject({ ok: false });
		expect(
			session.stagePrompt('oversized', {
				messages: [{ role: 'user', content: { type: 'text', text: 'x'.repeat(100 * 1024 + 1) } }]
			} as GetPromptResult)
		).toMatchObject({ ok: false, message: expect.stringContaining('100 KiB') });
		expect(session.stagePrompt('empty', { messages: [] } as GetPromptResult)).toMatchObject({
			ok: false
		});
		expect(session.stageResource('empty', { contents: [] } as ReadResourceResult)).toMatchObject({
			ok: false
		});
		expect(session.stagedContext).toHaveLength(2);
		session.close();
	});

	it('creates a fresh SDK connection when initialization is retried', async () => {
		let initializeAttempts = 0;
		const fetcher = readyFetch({}, (request) => {
			if (request.method === 'initialize' && initializeAttempts++ === 0) {
				return new Response('server unavailable', { status: 503 });
			}
		});
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			fetcher
		);

		await session.initialize();
		expect(session.status).toBe('unhealthy');

		await session.initialize(true);
		expect(session.status).toBe('ready');
		expect(initializeAttempts).toBe(2);
		session.close();
	});

	it('coordinates one workflow and cancels it during teardown', async () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			readyFetch()
		);
		await session.initialize();
		expect(session.status).toBe('ready');

		const workflow = session.beginWorkflow('direct', 'tool call');
		expect(() => session.beginWorkflow('chat', 'chat turn')).toThrow(/tool call is active/);
		session.close();

		expect(workflow.abort.signal.aborted).toBe(true);
		expect(session.status).toBe('closed');
		expect(session.activeWorkflow).toBeUndefined();
		expect(session.cache.tools.items).toEqual([]);
	});

	it('classifies setup, health, authentication, and access states', async () => {
		const setup = new MCPTesterSession(
			{ ...server, configured: false },
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>()
		);
		await setup.initialize();
		expect(setup.status).toBe('setup-required');

		const reauthenticate = new MCPTesterSession(
			{ ...server, configured: false, missingOAuthCredentials: true },
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>()
		);
		await reauthenticate.initialize();
		expect(reauthenticate.status).toBe('reauthentication-required');

		const unhealthy = new MCPTesterSession(
			{ ...server, deploymentStatus: 'Unavailable' },
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>()
		);
		await unhealthy.initialize();
		expect(unhealthy.status).toBe('unhealthy');

		const denied = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>().mockResolvedValue(new Response('forbidden', { status: 403 }))
		);
		await denied.initialize();
		expect(denied.status).toBe('access-denied');

		const expiredAuthentication = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>().mockResolvedValue(new Response('unauthorized', { status: 401 }))
		);
		await expiredAuthentication.initialize();
		expect(expiredAuthentication.status).toBe('reauthentication-required');
	});
	it('records the initialize handshake in both directions', async () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			readyFetch()
		);
		await session.initialize();

		const methods = session.log.entries.map((entry) => `${entry.direction} ${entry.method}`);
		expect(methods).toContain('outgoing initialize');
		expect(methods).toContain('outgoing notifications/initialized');

		const initializeRequest = session.log.entries.find(
			(entry) => entry.kind === 'request' && entry.method === 'initialize'
		);
		expect(initializeRequest?.outcome).toBe('ok');
		expect(initializeRequest?.durationMs).toBeGreaterThanOrEqual(0);

		const initializeResponse = session.log.entries.find(
			(entry) => entry.id === initializeRequest?.responseEntryId
		);
		expect(initializeResponse?.direction).toBe('incoming');
		expect(initializeResponse?.kind).toBe('response');

		session.close();
	});

	it('logs a tool call issued through the session', async () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			readyFetch({ tools: {} }, (request) =>
				request.method === 'tools/call'
					? Response.json({
							jsonrpc: '2.0',
							id: request.id,
							result: { content: [{ type: 'text', text: 'ok' }] }
						})
					: undefined
			)
		);
		await session.initialize();
		await session.callTool('lookup', { query: 'a' });

		const call = session.log.entries.find(
			(entry) => entry.kind === 'request' && entry.method === 'tools/call'
		);
		expect(call?.direction).toBe('outgoing');
		expect(call?.outcome).toBe('ok');

		session.close();
	});

	it('clears the log when the connection is rebuilt', async () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			readyFetch()
		);
		await session.initialize();
		const before = session.log.entries.map((entry) => entry.id);
		expect(before.length).toBeGreaterThan(0);

		await session.initialize(true);

		// Nothing from the previous connection survives, but the new handshake is present
		// and its ids never collide with the cleared ones.
		const after = session.log.entries;
		expect(after.some((entry) => before.includes(entry.id))).toBe(false);
		expect(after.some((entry) => entry.method === 'initialize')).toBe(true);

		session.close();
	});

	it('records a transport error when the connection fails', async () => {
		const session = new MCPTesterSession(
			server,
			{ name: 'obot-mcp-tester', version: 'test' },
			vi.fn<typeof fetch>().mockResolvedValue(new Response('boom', { status: 500 }))
		);
		await session.initialize();

		expect(session.status).toBe('error');
		expect(session.log.entries.some((entry) => entry.event === 'error')).toBe(true);
		// The outgoing frame is recorded before the transport throws.
		expect(session.log.entries.some((entry) => entry.method === 'initialize')).toBe(true);

		session.close();
	});
});
