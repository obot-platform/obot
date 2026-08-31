import type { MCPMessageDirection, MCPMessageSink } from './logging-transport';
import {
	isJSONRPCErrorResponse,
	isJSONRPCNotification,
	isJSONRPCRequest,
	isJSONRPCResultResponse,
	type JSONRPCMessage,
	type RequestId
} from '@modelcontextprotocol/sdk/types.js';

export const DEFAULT_LOG_CAPACITY = 500;

export type TesterLogDirection = MCPMessageDirection | 'local';
export type TesterLogKind =
	| 'request'
	| 'response'
	| 'error-response'
	| 'notification'
	| 'transport-event';

export interface TesterLogEntry {
	readonly id: number;
	readonly at: number;
	readonly mono: number;
	readonly direction: TesterLogDirection;
	readonly kind: TesterLogKind;
	readonly method?: string;
	readonly rpcId?: RequestId;
	readonly message?: JSONRPCMessage;
	readonly event?: 'connecting' | 'closed' | 'error';
	readonly errorMessage?: string;
	readonly responseEntryId?: number;
	readonly requestEntryId?: number;
	readonly durationMs?: number;
	readonly outcome?: 'ok' | 'error' | 'cancelled';
}

// Ids may be number or string (1 and "1" are distinct), and each party numbers its
// requests independently, so an inbound id can collide with an outbound one.
function correlationKey(requestDirection: TesterLogDirection, id: RequestId): string {
	return `${requestDirection}:${typeof id}:${id}`;
}

function classify(message: JSONRPCMessage): TesterLogKind {
	if (isJSONRPCRequest(message)) return 'request';
	if (isJSONRPCNotification(message)) return 'notification';
	if (isJSONRPCErrorResponse(message)) return 'error-response';
	if (isJSONRPCResultResponse(message)) return 'response';
	return 'transport-event';
}

function cancelledRequestId(message: JSONRPCMessage): RequestId | undefined {
	if (!isJSONRPCNotification(message) || message.method !== 'notifications/cancelled') {
		return undefined;
	}
	const requestId = (message.params as { requestId?: unknown } | undefined)?.requestId;
	return typeof requestId === 'string' || typeof requestId === 'number' ? requestId : undefined;
}

export class MCPTesterLog implements MCPMessageSink {
	// $state.raw, not deep $state: entries are immutable once appended and hold large
	// JSON payloads that deep reactivity would recursively proxy on every read.
	entries = $state.raw<TesterLogEntry[]>([]);
	dropped = $state(0);

	readonly #capacity: number;
	// Correlation bookkeeping, not reactive UI state.
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	#pending = new Map<string, TesterLogEntry>();
	#nextId = 1;

	constructor(capacity: number = DEFAULT_LOG_CAPACITY) {
		this.#capacity = capacity;
	}

	get length(): number {
		return this.entries.length;
	}

	recordMessage(direction: MCPMessageDirection, message: JSONRPCMessage): void {
		const kind = classify(message);
		const rpcId = 'id' in message ? message.id : undefined;
		const entry: TesterLogEntry = {
			id: this.#nextId++,
			at: Date.now(),
			mono: performance.now(),
			direction,
			kind,
			rpcId,
			method: 'method' in message ? message.method : undefined,
			message
		};

		if (kind === 'request' && rpcId !== undefined) {
			this.#commit(entry);
			this.#pending.set(correlationKey(direction, rpcId), entry);
			return;
		}

		// Protocol sends notifications/cancelled when a workflow is aborted; pair it
		// back to the in-flight request so the UI can label it rather than leaving an orphan.
		const cancelled = cancelledRequestId(message);
		if (cancelled !== undefined) {
			const key = correlationKey(direction, cancelled);
			const request = this.#pending.get(key);
			if (request) {
				this.#pending.delete(key);
				this.#commit(entry, { ...request, outcome: 'cancelled' });
				return;
			}
		}

		if ((kind === 'response' || kind === 'error-response') && rpcId !== undefined) {
			const requestDirection: MCPMessageDirection =
				direction === 'incoming' ? 'outgoing' : 'incoming';
			const key = correlationKey(requestDirection, rpcId);
			const request = this.#pending.get(key);
			if (request) {
				this.#pending.delete(key);
				const durationMs = entry.mono - request.mono;
				this.#commit(
					{ ...entry, requestEntryId: request.id, durationMs, method: request.method },
					{
						...request,
						responseEntryId: entry.id,
						durationMs,
						outcome: kind === 'error-response' ? 'error' : 'ok'
					}
				);
				return;
			}
		}

		this.#commit(entry);
	}

	recordLifecycle(event: 'connecting' | 'closed'): void {
		this.#commit({
			id: this.#nextId++,
			at: Date.now(),
			mono: performance.now(),
			direction: 'local',
			kind: 'transport-event',
			event
		});
	}

	recordError(error: Error): void {
		this.#commit({
			id: this.#nextId++,
			at: Date.now(),
			mono: performance.now(),
			direction: 'local',
			kind: 'transport-event',
			event: 'error',
			errorMessage: error.message || String(error)
		});
	}

	clear(): void {
		this.entries = [];
		this.#pending.clear();
		this.dropped = 0;
		// #nextId is deliberately NOT reset: ids stay globally unique so a keyed
		// {#each} can never reuse DOM across a reconnect.
	}

	toNDJSON(entries: TesterLogEntry[] = this.entries): string {
		return entries
			.map((entry) =>
				JSON.stringify({
					id: entry.id,
					// Transient formatting of an immutable timestamp, not reactive state.
					// eslint-disable-next-line svelte/prefer-svelte-reactivity
					at: new Date(entry.at).toISOString(),
					direction: entry.direction,
					kind: entry.kind,
					method: entry.method,
					rpcId: entry.rpcId,
					durationMs: entry.durationMs,
					outcome: entry.outcome,
					event: entry.event,
					errorMessage: entry.errorMessage,
					message: entry.message
				})
			)
			.join('\n');
	}

	#commit(entry: TesterLogEntry, patched?: TesterLogEntry): void {
		const next = this.entries.slice();
		if (patched) {
			const index = next.findIndex((candidate) => candidate.id === patched.id);
			if (index >= 0) next[index] = patched;
		}
		next.push(entry);
		if (next.length > this.#capacity) {
			for (const evicted of next.splice(0, next.length - this.#capacity)) {
				this.dropped += 1;
				// Keep the pending map bounded by the ring buffer.
				if (
					evicted.kind === 'request' &&
					evicted.rpcId !== undefined &&
					evicted.responseEntryId === undefined &&
					evicted.direction !== 'local'
				) {
					this.#pending.delete(correlationKey(evicted.direction, evicted.rpcId));
				}
			}
		}
		this.entries = next;
	}
}
