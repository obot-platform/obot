import { LoggingTransport, type MCPMessageSink } from './logging-transport';
import type { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it, vi } from 'vitest';

function setup() {
	const inner = {
		send: vi.fn().mockResolvedValue(undefined),
		close: vi.fn().mockResolvedValue(undefined)
	} as unknown as StreamableHTTPClientTransport;
	const sink: MCPMessageSink = {
		recordMessage: vi.fn(),
		recordLifecycle: vi.fn(),
		recordError: vi.fn()
	};
	const transport = new LoggingTransport(inner, sink);
	const receive = (message: JSONRPCMessage) => inner.onmessage?.(message);
	return { transport, receive, sink };
}

describe('LoggingTransport response capture', () => {
	it('matches the outgoing method and typed id, before forwarding the response to the SDK', async () => {
		const { transport, receive } = setup();
		const capture = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 1, method: 'tools/list' });
		await transport.send({ jsonrpc: '2.0', id: 2, method: 'tools/call' });
		receive({ jsonrpc: '2.0', id: 1, result: {} });
		receive({ jsonrpc: '2.0', id: '2', result: {} });
		receive({ jsonrpc: '2.0', id: 2, method: 'sampling/createMessage', params: {} });
		receive({ jsonrpc: '2.0', method: 'notifications/message', params: {} });
		await transport.send({ jsonrpc: '2.0', id: 2, result: {} });
		expect(capture.response).toBeUndefined();
		const response = { jsonrpc: '2.0' as const, id: 2, result: { content: [] } };
		transport.onmessage = () => expect(capture.response).toEqual(response);
		receive(response);
		expect(capture.response).toEqual(response);
	});

	it('keeps interleaved responses with their own requests and retains error envelopes', async () => {
		const { transport, receive } = setup();
		const first = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 1, method: 'tools/call' });
		const second = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 2, method: 'tools/call' });
		const error = {
			jsonrpc: '2.0' as const,
			id: 2,
			error: { code: -32602, message: 'Invalid arguments', data: { field: 'query' } }
		};
		receive(error);
		expect(first.response).toBeUndefined();
		expect(second.response).toEqual(error);
		receive({ jsonrpc: '2.0', id: 1, result: { content: [] } });
		expect(first.response?.id).toBe(1);
		expect(second.response).toEqual(error);
	});

	it('does not let a disposed capture claim a later request or response', async () => {
		const { transport, receive } = setup();
		const unsent = transport.captureResponse('tools/call');
		unsent.dispose();
		const cancelled = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 1, method: 'tools/call' });
		cancelled.dispose();
		const next = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 2, method: 'tools/call' });
		receive({ jsonrpc: '2.0', id: 1, result: {} });
		expect(next.response).toBeUndefined();
		receive({ jsonrpc: '2.0', id: 2, result: {} });
		expect(unsent.response).toBeUndefined();
		expect(cancelled.response).toBeUndefined();
		expect(next.response?.id).toBe(2);
	});

	it.each(['detach', 'close'] as const)('discards pending captures on %s', async (action) => {
		const { transport, receive } = setup();
		const capture = transport.captureResponse('tools/call');
		await transport.send({ jsonrpc: '2.0', id: 1, method: 'tools/call' });
		await transport[action]();
		receive({ jsonrpc: '2.0', id: 1, result: {} });
		expect(capture.response).toBeUndefined();
	});
});
