import { page as appPage } from '$app/state';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { createMcpServerDetailsFixtures } from '../../../../tests/mocks/data';
import { worker } from '../../../../tests/mocks/worker';
import type { PageData } from './$types';
import TesterPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const fixtures = createMcpServerDetailsFixtures();

interface MCPRequest {
	id?: string | number;
	method: string;
	params?: Record<string, unknown>;
}

function mockMCPInitialization(
	capabilities: Record<string, unknown> = {},
	handleRequest?: (request: MCPRequest) => unknown
) {
	worker.use(
		http.post(`/mcp-connect/${fixtures.serverSingle.id}`, async ({ request }) => {
			const body = (await request.json()) as MCPRequest;
			if (body.method === 'initialize') {
				return HttpResponse.json(
					{
						jsonrpc: '2.0',
						id: body.id,
						result: {
							protocolVersion: body.params?.protocolVersion,
							capabilities,
							serverInfo: { name: 'tester-server', version: '1.0.0' }
						}
					},
					{ headers: { 'Mcp-Session-Id': 'tester-session' } }
				);
			}
			const result = handleRequest?.(body);
			if (result !== undefined) {
				return HttpResponse.json({ jsonrpc: '2.0', id: body.id, result });
			}
			return new HttpResponse(null, { status: 202 });
		}),
		http.get(`/mcp-connect/${fixtures.serverSingle.id}`, () => {
			return new HttpResponse(null, { status: 405 });
		})
	);
}

function mockMCPInitializationFailure(status = 500) {
	worker.use(
		http.post(`/mcp-connect/${fixtures.serverSingle.id}`, () => new HttpResponse(null, { status })),
		http.get(
			`/mcp-connect/${fixtures.serverSingle.id}`,
			() => new HttpResponse(null, { status: 405 })
		)
	);
}

async function renderTester(
	tab?: string,
	capabilities: Record<string, unknown> = {},
	handleRequest?: (request: MCPRequest) => unknown,
	pageOverrides: Partial<PageData> = {}
) {
	mockMCPInitialization(capabilities, handleRequest);
	if (tab) {
		appPage.url.searchParams.set('tab', tab);
	} else {
		appPage.url.searchParams.delete('tab');
	}
	const data = await preparePageData<PageData>({
		server: {
			...fixtures.serverSingle,
			configured: true,
			deploymentStatus: 'Available',
			canConnect: true
		},
		backTarget: `/mcp-servers/c/${fixtures.entrySingle.id}/instance/${fixtures.serverSingle.id}`,
		...pageOverrides
	});
	return render(TesterPage, { data });
}

const chatModelData: Partial<PageData> = {
	defaultModelAliases: [{ alias: 'llm', model: 'm1tester' }],
	models: [
		{
			id: 'm1tester',
			active: true,
			aliasAssigned: true,
			created: 1,
			modelProvider: 'mp1tester',
			modelProviderName: 'Test Provider',
			name: 'test-model',
			displayName: 'Test Model',
			targetModel: 'test-model',
			usage: 'llm'
		}
	]
};

function chatStream(...events: unknown[]) {
	return new HttpResponse(events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join(''), {
		headers: { 'Content-Type': 'text/event-stream' }
	});
}

describe('MCP Tester page', () => {
	it('initializes the shell and defaults an invalid tab to Chat', async () => {
		await renderTester('not-a-tab');

		await expect.element(page.getByRole('heading', { name: 'MCP Tester' })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: fixtures.serverSingle.manifest.name }))
			.toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Chat', exact: true })).toBeVisible();
		await expect.element(page.getByText('Chat unavailable', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: `Back to ${fixtures.serverSingle.manifest.name}` }))
			.toHaveAttribute(
				'href',
				`/mcp-servers/c/${fixtures.entrySingle.id}/instance/${fixtures.serverSingle.id}`
			);
	});

	it('uses a valid tab query value', async () => {
		await renderTester('tools');
		await expect.element(page.getByRole('heading', { name: 'Tools', exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Tools', exact: true }))
			.toHaveAttribute('aria-current', 'page');
	});

	it('keeps the Chat composer visible while messages scroll inside the chat', async () => {
		await renderTester('chat', {}, undefined, chatModelData);

		const messages = page.getByLabelText('Chat messages');
		const composer = page.getByRole('region', { name: 'Chat composer' });
		await expect.element(messages).toBeVisible();
		await expect.element(composer).toBeVisible();

		const messagesElement = messages.element();
		const composerBounds = composer.element().getBoundingClientRect();
		expect(getComputedStyle(messagesElement).overflowY).toBe('auto');
		expect(messagesElement.getBoundingClientRect().bottom).toBeLessThanOrEqual(composerBounds.top);
		expect(composerBounds.bottom).toBeLessThanOrEqual(window.innerHeight);
	});

	it('sends with Enter and safely renders streamed Markdown', async () => {
		const requests = vi.fn();
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, async ({ request }) => {
				requests(await request.json());
				return chatStream(
					{ type: 'assistant_message_start' },
					{
						type: 'text_delta',
						delta: '**Connected** [docs](https://example.com) <script>unsafe()</script>'
					},
					{ type: 'completion', reason: 'stop' }
				);
			})
		);
		await renderTester('chat', {}, undefined, chatModelData);

		const composer = page.getByRole('textbox', { name: 'Message', exact: true });
		await composer.fill('Hello tester');
		composer
			.element()
			.dispatchEvent(
				new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
			);

		await expect.element(page.getByText('Connected', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'docs' }))
			.toHaveAttribute('rel', 'noopener noreferrer');
		await expect.element(page.getByText('unsafe()', { exact: true })).not.toBeInTheDocument();
		await vi.waitFor(() =>
			expect(requests).toHaveBeenCalledWith(
				expect.objectContaining({
					messages: [{ role: 'user', content: [{ type: 'text', text: 'Hello tester' }] }],
					tools: [],
					round: 1
				})
			)
		);
	});

	it('re-enables the composer after the assistant response completes', async () => {
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, () =>
				chatStream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'First response complete.' },
					{ type: 'completion', reason: 'stop' }
				)
			)
		);
		await renderTester('chat', {}, undefined, chatModelData);

		const composer = page.getByRole('textbox', { name: 'Message', exact: true });
		await composer.fill('First message');
		await page.getByRole('button', { name: 'Send', exact: true }).click();

		await expect.element(page.getByText('First response complete.', { exact: true })).toBeVisible();
		await expect.element(composer).toBeEnabled();
		await composer.fill('Follow-up message');
		await expect.element(page.getByRole('button', { name: 'Send', exact: true })).toBeEnabled();
	});

	it('keeps a partial failure visible and replaces it on Retry', async () => {
		let attempts = 0;
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, () => {
				attempts += 1;
				return attempts === 1
					? chatStream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'Partial response' },
							{
								type: 'error',
								error: {
									code: 'provider_error',
									message: 'Provider failed',
									retryable: true
								}
							}
						)
					: chatStream(
							{ type: 'assistant_message_start' },
							{ type: 'text_delta', delta: 'Replacement response' },
							{ type: 'completion', reason: 'stop' }
						);
			})
		);
		await renderTester('chat', {}, undefined, chatModelData);

		await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Retry this');
		await page.getByRole('button', { name: 'Send', exact: true }).click();
		await expect.element(page.getByText('Partial response', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Provider failed', { exact: true })).toBeVisible();

		await page.getByRole('button', { name: 'Retry response' }).click();
		await expect.element(page.getByText('Replacement response', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Partial response', { exact: true }))
			.not.toBeInTheDocument();
		expect(attempts).toBe(2);
	});

	it('approves a read-only tool request and completes the same turn with its result', async () => {
		const modelRequests: unknown[] = [];
		const toolCalls = vi.fn();
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, async ({ request }) => {
				modelRequests.push(await request.json());
				if (modelRequests.length === 1) {
					return chatStream(
						{ type: 'assistant_message_start' },
						{
							type: 'tool_calls',
							calls: [{ id: 'call-lookup', name: 'lookup', arguments: { query: 'safe' } }]
						},
						{ type: 'completion', reason: 'tool_calls' }
					);
				}
				return chatStream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'The lookup returned one result.' },
					{ type: 'completion', reason: 'stop' }
				);
			})
		);
		await renderTester(
			'chat',
			{ tools: {} },
			(request) => {
				if (request.method === 'tools/list') {
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
				if (request.method === 'tools/call') {
					toolCalls(request.params);
					return {
						content: [{ type: 'text', text: 'one result' }],
						isError: false
					};
				}
			},
			chatModelData
		);

		await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Run lookup');
		await page.getByRole('button', { name: 'Send', exact: true }).click();

		await expect.element(page.getByText('Approval needed', { exact: true }).last()).toBeVisible();
		await expect.element(page.getByRole('link', { name: /Chat Approval needed/ })).toBeVisible();
		await expect
			.element(page.getByLabelText('lookup arguments'))
			.toHaveTextContent('"query": "safe"');
		await expect
			.element(page.getByRole('button', { name: /Always allow/ }))
			.not.toBeInTheDocument();
		const approve = page.getByRole('button', { name: 'Approve lookup' });
		await expect.element(approve).toHaveFocus();
		await approve.click();

		await expect
			.element(page.getByText('The lookup returned one result.', { exact: true }))
			.toBeVisible();
		await vi.waitFor(() =>
			expect(toolCalls).toHaveBeenCalledWith({
				name: 'lookup',
				arguments: { query: 'safe' }
			})
		);
		expect(modelRequests).toHaveLength(2);
		expect(modelRequests[1]).toMatchObject({
			messages: [
				{ role: 'user' },
				{ role: 'assistant', toolCalls: [{ id: 'call-lookup', name: 'lookup' }] },
				{
					role: 'tool',
					toolResult: { callID: 'call-lookup', status: 'success' }
				}
			],
			round: 2
		});
	});

	it('approves batched requests one at a time and records each result in the transcript', async () => {
		const modelRequests: unknown[] = [];
		const executionOrder: string[] = [];
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, async ({ request }) => {
				modelRequests.push(await request.json());
				if (modelRequests.length === 1) {
					return chatStream(
						{ type: 'assistant_message_start' },
						{
							type: 'tool_calls',
							calls: [
								{ id: 'call-first', name: 'first', arguments: { order: 1 } },
								{ id: 'call-second', name: 'second', arguments: { order: 2 } }
							]
						},
						{ type: 'completion', reason: 'tool_calls' }
					);
				}
				return chatStream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'Both tools completed.' },
					{ type: 'completion', reason: 'stop' }
				);
			})
		);
		await renderTester(
			'chat',
			{ tools: {} },
			(request) => {
				if (request.method === 'tools/list') {
					return {
						tools: [
							{ name: 'first', inputSchema: { type: 'object' } },
							{ name: 'second', inputSchema: { type: 'object' } }
						]
					};
				}
				if (request.method === 'tools/call') {
					const name = String(request.params?.name);
					executionOrder.push(name);
					return {
						content: [{ type: 'text', text: `${name} result content` }],
						isError: false
					};
				}
			},
			chatModelData
		);

		await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Run both');
		await page.getByRole('button', { name: 'Send', exact: true }).click();
		// Only the head of the queue is offered; the rest wait their turn.
		await expect
			.element(page.getByRole('button', { name: 'Approve second' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Approve first' }).click();

		await expect.element(page.getByRole('article', { name: 'first tool call' })).toBeVisible();
		expect(executionOrder).toEqual(['first']);
		await page.getByRole('button', { name: 'Approve second' }).click();

		await expect.element(page.getByText('Both tools completed.', { exact: true })).toBeVisible();
		expect(executionOrder).toEqual(['first', 'second']);
		await expect
			.element(
				page
					.getByRole('article', { name: 'first tool call' })
					.getByRole('region', { name: 'first result' })
			)
			.toHaveTextContent('first result content');
		await expect
			.element(
				page
					.getByRole('article', { name: 'second tool call' })
					.getByRole('region', { name: 'second result' })
			)
			.toHaveTextContent('second result content');
		await expect
			.element(page.getByRole('article', { name: 'Tool result' }))
			.not.toBeInTheDocument();
	});

	it('confirms New Chat before clearing an existing in-memory conversation', async () => {
		worker.use(
			http.post(`/api/mcp-servers/${fixtures.serverSingle.id}/tester/chat`, () =>
				chatStream(
					{ type: 'assistant_message_start' },
					{ type: 'text_delta', delta: 'Temporary answer' },
					{ type: 'completion', reason: 'stop' }
				)
			)
		);
		await renderTester('chat', {}, undefined, chatModelData);
		await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Temporary question');
		await page.getByRole('button', { name: 'Send', exact: true }).click();
		await expect.element(page.getByText('Temporary answer', { exact: true })).toBeVisible();

		await page.getByRole('button', { name: 'New Chat' }).click();
		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(page.getByText('Clear this ephemeral conversation?')).toBeVisible();
		await page.getByRole('button', { name: 'Start New Chat' }).click();

		await expect
			.element(page.getByText('Temporary answer', { exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText('Send a message or stage a prompt or text resource to begin.'))
			.toBeVisible();
	});

	it('discovers, searches, validates, and directly calls tools', async () => {
		const toolCall = vi.fn();
		await renderTester('tools', { tools: {} }, (request) => {
			if (request.method === 'tools/list') {
				return {
					tools: [
						{
							name: 'lookup',
							description: 'Look up a record',
							inputSchema: {
								type: 'object',
								required: ['query'],
								properties: {
									query: { type: 'string', minLength: 2 },
									limit: { type: 'integer', minimum: 1 }
								}
							},
							outputSchema: {
								type: 'object',
								properties: { found: { type: 'boolean' } }
							}
						}
					]
				};
			}
			if (request.method === 'tools/call') {
				toolCall(request.params);
				return {
					content: [{ type: 'text', text: 'Found safely <script>never runs</script>' }],
					structuredContent: { found: true },
					isError: false
				};
			}
		});

		await page.getByRole('button', { name: /lookup/ }).click();
		await page.getByLabelText('query *').fill('record');
		await page.getByLabelText('limit').fill('2');
		await page.getByRole('button', { name: 'Call', exact: true }).click();

		await vi.waitFor(() =>
			expect(toolCall).toHaveBeenCalledWith({
				name: 'lookup',
				arguments: { query: 'record', limit: 2 }
			})
		);
		await expect.element(page.getByText('Succeeded')).toBeVisible();
		await expect
			.element(page.getByText('Found safely <script>never runs</script>', { exact: true }).first())
			.toBeVisible();
		await expect.element(page.getByText('Structured content')).toBeVisible();
	});

	it('resolves prompt arguments and preserves message roles and order', async () => {
		await renderTester('prompts', { prompts: {} }, (request) => {
			if (request.method === 'prompts/list') {
				return {
					prompts: [
						{
							name: 'explain',
							description: 'Explain a topic',
							arguments: [{ name: 'topic', required: true }]
						}
					]
				};
			}
			if (request.method === 'prompts/get') {
				return {
					messages: [
						{ role: 'user', content: { type: 'text', text: 'First message' } },
						{ role: 'assistant', content: { type: 'text', text: 'Second message' } }
					]
				};
			}
		});

		await page.getByRole('button', { name: /explain/ }).click();
		await page.getByLabelText('topic *').fill('MCP');
		await page.getByRole('button', { name: 'Get prompt' }).click();

		await expect.element(page.getByText('First message', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByText('Second message', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByText('user', { exact: true })).toBeVisible();
		await expect.element(page.getByText('assistant', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Use in Chat' })).toBeVisible();
	});

	it('reads text resources on selection and keeps binary resources inspectable only', async () => {
		await renderTester('resources', { resources: {} }, (request) => {
			if (request.method === 'resources/list') {
				return {
					resources: [
						{ uri: 'file:///guide.txt', name: 'Guide', mimeType: 'text/plain' },
						{ uri: 'file:///image.bin', name: 'Binary', mimeType: 'application/octet-stream' }
					]
				};
			}
			if (request.method === 'resources/read') {
				return request.params?.uri === 'file:///guide.txt'
					? {
							contents: [
								{ uri: 'file:///guide.txt', mimeType: 'text/plain', text: 'Resource body' }
							]
						}
					: {
							contents: [
								{
									uri: 'file:///image.bin',
									mimeType: 'application/octet-stream',
									blob: 'AAAA'
								}
							]
						};
			}
		});

		await page.getByRole('button', { name: /Guide/ }).click();
		await expect.element(page.getByText('Resource body', { exact: true }).first()).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Use in Chat' })).toBeVisible();

		await page.getByRole('button', { name: /Binary/ }).click();
		await expect
			.element(page.getByText(/Binary or unsupported content remains inspectable/))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Use in Chat' })).not.toBeInTheDocument();
	});

	it('shows setup guidance without opening an MCP session', async () => {
		appPage.url.searchParams.delete('tab');
		const data = await preparePageData<PageData>({
			server: {
				...fixtures.serverSingle,
				configured: false,
				canConnect: true
			},
			backTarget: `/mcp-servers/s/${fixtures.serverSingle.id}`
		});
		render(TesterPage, { data });

		await expect
			.element(page.getByRole('heading', { name: 'Server setup required' }))
			.toBeVisible();
		await expect.element(page.getByRole('link', { name: 'Manage server' })).toBeVisible();
	});
	it('exposes an MCP Log tab in the section nav', async () => {
		await renderTester();
		await expect.element(page.getByRole('link', { name: 'MCP Log', exact: true })).toBeVisible();
	});

	it('marks MCP Log as the current tab and renders the traffic log', async () => {
		await renderTester('logs');

		await expect
			.element(page.getByRole('link', { name: 'MCP Log', exact: true }))
			.toHaveAttribute('aria-current', 'page');
		await expect.element(page.getByRole('list', { name: 'MCP traffic log' })).toBeVisible();
	});

	it('records the initialize handshake in both directions', async () => {
		await renderTester('logs');

		await expect
			.element(page.getByRole('button', { name: /Sent request initialize/ }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Received response initialize/ }))
			.toBeVisible();
	});

	it('keeps the MCP Log tab usable when the session fails to connect', async () => {
		mockMCPInitializationFailure();
		appPage.url.searchParams.set('tab', 'logs');
		const data = await preparePageData<PageData>({
			server: {
				...fixtures.serverSingle,
				configured: true,
				deploymentStatus: 'Available',
				canConnect: true
			},
			backTarget: `/mcp-servers/s/${fixtures.serverSingle.id}`
		});
		render(TesterPage, { data });

		await expect.element(page.getByRole('alert')).toHaveTextContent('Connection failed');
		// The failed handshake is inspectable even though the session never connected.
		await expect
			.element(page.getByRole('button', { name: /transport .*Streamable HTTP error/ }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Retry' })).toBeVisible();
		// The status card belongs to the other tabs; the log surfaces status inline instead.
		await expect
			.element(page.getByRole('heading', { name: 'Server unavailable' }))
			.not.toBeInTheDocument();
	});

	it('still shows the log for a server that was never dialled', async () => {
		appPage.url.searchParams.set('tab', 'logs');
		const data = await preparePageData<PageData>({
			server: {
				...fixtures.serverSingle,
				configured: false,
				canConnect: true
			},
			backTarget: `/mcp-servers/s/${fixtures.serverSingle.id}`
		});
		render(TesterPage, { data });

		await expect.element(page.getByText('this server still needs', { exact: false })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Server setup required' }))
			.not.toBeInTheDocument();
	});
});
