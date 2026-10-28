import type { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import type {
	Transport,
	TransportSendOptions
} from '@modelcontextprotocol/sdk/shared/transport.js';
import {
	isJSONRPCErrorResponse,
	isJSONRPCRequest,
	isJSONRPCResultResponse,
	type JSONRPCMessage,
	type JSONRPCErrorResponse,
	type JSONRPCResultResponse,
	type MessageExtraInfo,
	type RequestId
} from '@modelcontextprotocol/sdk/types.js';

export type MCPRawResponse = JSONRPCResultResponse | JSONRPCErrorResponse;

interface ResponseCapture {
	method: string;
	requestId?: RequestId;
	response?: MCPRawResponse;
	dispose(): void;
}

export type MCPMessageDirection = 'outgoing' | 'incoming';

export interface MCPMessageSink {
	recordMessage(direction: MCPMessageDirection, message: JSONRPCMessage): void;
	recordLifecycle(event: 'connecting' | 'closed'): void;
	recordError(error: Error): void;
}

export class LoggingTransport implements Transport {
	readonly inner: StreamableHTTPClientTransport;
	#sink: MCPMessageSink | undefined;
	#captures = new Set<ResponseCapture>();

	onclose?: () => void;
	onerror?: (error: Error) => void;
	onmessage?: (message: JSONRPCMessage, extra?: MessageExtraInfo) => void;

	constructor(inner: StreamableHTTPClientTransport, sink: MCPMessageSink) {
		this.inner = inner;
		this.#sink = sink;
		inner.onmessage = ((message: JSONRPCMessage, extra?: MessageExtraInfo) => {
			if (isJSONRPCResultResponse(message) || isJSONRPCErrorResponse(message)) {
				for (const capture of this.#captures) {
					if (capture.requestId === message.id) {
						capture.response = message;
						capture.dispose();
					}
				}
			}
			this.#sink?.recordMessage('incoming', message);
			this.onmessage?.(message, extra);
		}) as StreamableHTTPClientTransport['onmessage'];
		inner.onerror = (error) => {
			this.#sink?.recordError(error);
			this.onerror?.(error);
		};
		inner.onclose = () => {
			this.#captures.clear();
			this.#sink?.recordLifecycle('closed');
			this.onclose?.();
		};
	}

	detach(): void {
		this.#sink = undefined;
		this.#captures.clear();
	}

	// Register immediately before invoking the SDK. The tester serializes tool calls;
	// bind to the outgoing request id rather than guessing from the latest log entry.
	captureResponse(method: string): ResponseCapture {
		const capture: ResponseCapture = {
			method,
			dispose: () => this.#captures.delete(capture)
		};
		this.#captures.add(capture);
		return capture;
	}

	async start(): Promise<void> {
		this.#sink?.recordLifecycle('connecting');
		await this.inner.start();
	}

	async send(message: JSONRPCMessage, options?: TransportSendOptions): Promise<void> {
		if (isJSONRPCRequest(message)) {
			for (const capture of this.#captures) {
				if (capture.requestId === undefined && capture.method === message.method) {
					capture.requestId = message.id;
					break;
				}
			}
		}
		this.#sink?.recordMessage('outgoing', message);
		await this.inner.send(message, options);
	}

	close(): Promise<void> {
		this.#captures.clear();
		return this.inner.close();
	}

	get sessionId(): string | undefined {
		return this.inner.sessionId;
	}

	setProtocolVersion(version: string): void {
		this.inner.setProtocolVersion(version);
	}
}
