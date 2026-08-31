import { DEFAULT_LOG_CAPACITY, MCPTesterLog } from './tester-log.svelte';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it } from 'vitest';

function request(id: string | number, method: string, params?: Record<string, unknown>) {
	return { jsonrpc: '2.0', id, method, ...(params ? { params } : {}) } as JSONRPCMessage;
}

function response(id: string | number, result: Record<string, unknown> = {}) {
	return { jsonrpc: '2.0', id, result } as JSONRPCMessage;
}

function errorResponse(id: string | number, code = -32603, message = 'boom') {
	return { jsonrpc: '2.0', id, error: { code, message } } as JSONRPCMessage;
}

function notification(method: string, params?: Record<string, unknown>) {
	return { jsonrpc: '2.0', method, ...(params ? { params } : {}) } as JSONRPCMessage;
}

describe('MCPTesterLog', () => {
	it('classifies each JSON-RPC frame shape', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/list'));
		log.recordMessage('incoming', response(1));
		log.recordMessage('incoming', notification('notifications/message'));
		log.recordMessage('outgoing', request(2, 'tools/call'));
		log.recordMessage('incoming', errorResponse(2));

		expect(log.entries.map((entry) => entry.kind)).toEqual([
			'request',
			'response',
			'notification',
			'request',
			'error-response'
		]);
	});

	it('correlates a response back to its request and records a duration on both halves', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/call', { name: 'lookup' }));
		log.recordMessage('incoming', response(1, { content: [] }));

		const [requestEntry, responseEntry] = log.entries;
		expect(requestEntry.responseEntryId).toBe(responseEntry.id);
		expect(responseEntry.requestEntryId).toBe(requestEntry.id);
		expect(requestEntry.outcome).toBe('ok');
		expect(requestEntry.durationMs).toBeGreaterThanOrEqual(0);
		expect(responseEntry.durationMs).toBe(requestEntry.durationMs);
		// The bare {id, result} frame inherits the method so the row can be labelled.
		expect(responseEntry.method).toBe('tools/call');
	});

	it('marks a correlated error response as a failed outcome', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(7, 'resources/read'));
		log.recordMessage('incoming', errorResponse(7));

		expect(log.entries[0].outcome).toBe('error');
		expect(log.entries[1].kind).toBe('error-response');
	});

	it('leaves an unanswered request without a duration', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/list'));

		expect(log.entries[0].durationMs).toBeUndefined();
		expect(log.entries[0].responseEntryId).toBeUndefined();
	});

	it('does not confuse a numeric id with the equivalent string id', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/list'));
		log.recordMessage('incoming', response('1'));

		// The string-keyed response matches no outgoing request, so nothing correlates.
		expect(log.entries[0].responseEntryId).toBeUndefined();
		expect(log.entries[1].requestEntryId).toBeUndefined();
	});

	it('does not correlate an inbound request with an outbound one sharing an id', () => {
		const log = new MCPTesterLog();
		log.recordMessage('incoming', request(1, 'sampling/createMessage'));
		log.recordMessage('incoming', response(1));

		// A response arriving inbound answers an outbound request; the inbound request
		// with the same id must not be claimed by it.
		expect(log.entries[0].responseEntryId).toBeUndefined();
		expect(log.entries[1].requestEntryId).toBeUndefined();
	});

	it('pairs a cancellation notification back to the in-flight request', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(3, 'tools/call'));
		log.recordMessage('outgoing', notification('notifications/cancelled', { requestId: 3 }));

		expect(log.entries[0].outcome).toBe('cancelled');
	});

	it('evicts the oldest entries once capacity is exceeded and counts the drops', () => {
		const log = new MCPTesterLog(3);
		for (let index = 1; index <= 5; index++) {
			log.recordMessage('outgoing', notification(`notifications/n${index}`));
		}

		expect(log.entries).toHaveLength(3);
		expect(log.entries.map((entry) => entry.method)).toEqual([
			'notifications/n3',
			'notifications/n4',
			'notifications/n5'
		]);
		expect(log.dropped).toBe(2);
	});

	it('drops pending correlations along with their evicted request', () => {
		const log = new MCPTesterLog(2);
		log.recordMessage('outgoing', request(1, 'tools/list'));
		log.recordMessage('outgoing', notification('notifications/a'));
		log.recordMessage('outgoing', notification('notifications/b'));
		// Request #1 has been evicted, so a late response must not correlate to it.
		log.recordMessage('incoming', response(1));

		expect(log.entries.at(-1)?.requestEntryId).toBeUndefined();
	});

	it('records transport lifecycle and error events', () => {
		const log = new MCPTesterLog();
		log.recordLifecycle('connecting');
		log.recordError(new Error('fetch failed'));
		log.recordLifecycle('closed');

		expect(log.entries.map((entry) => entry.event)).toEqual(['connecting', 'error', 'closed']);
		expect(log.entries.every((entry) => entry.kind === 'transport-event')).toBe(true);
		expect(log.entries[1].errorMessage).toBe('fetch failed');
	});

	it('clears entries but keeps ids monotonic across a reconnect', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/list'));
		const firstId = log.entries[0].id;
		log.clear();

		expect(log.entries).toHaveLength(0);
		expect(log.dropped).toBe(0);

		log.recordMessage('outgoing', request(1, 'tools/list'));
		expect(log.entries[0].id).toBeGreaterThan(firstId);
	});

	it('serialises the given entries as one JSON object per line', () => {
		const log = new MCPTesterLog();
		log.recordMessage('outgoing', request(1, 'tools/list'));
		log.recordMessage('incoming', response(1));

		const lines = log.toNDJSON().split('\n');
		expect(lines).toHaveLength(2);
		const parsed = lines.map((line) => JSON.parse(line) as Record<string, unknown>);
		expect(parsed[0].method).toBe('tools/list');
		expect(parsed[0].direction).toBe('outgoing');
		expect(parsed[1].direction).toBe('incoming');
	});

	it('defaults to a bounded capacity', () => {
		expect(DEFAULT_LOG_CAPACITY).toBe(500);
	});
});
