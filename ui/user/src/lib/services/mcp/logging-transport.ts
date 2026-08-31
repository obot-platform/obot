import type { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import type {
	Transport,
	TransportSendOptions
} from '@modelcontextprotocol/sdk/shared/transport.js';
import type { JSONRPCMessage, MessageExtraInfo } from '@modelcontextprotocol/sdk/types.js';

export type MCPMessageDirection = 'outgoing' | 'incoming';

export interface MCPMessageSink {
	recordMessage(direction: MCPMessageDirection, message: JSONRPCMessage): void;
	recordLifecycle(event: 'connecting' | 'closed'): void;
	recordError(error: Error): void;
}

export class LoggingTransport implements Transport {
	readonly inner: StreamableHTTPClientTransport;
	#sink: MCPMessageSink | undefined;

	onclose?: () => void;
	onerror?: (error: Error) => void;
	onmessage?: (message: JSONRPCMessage, extra?: MessageExtraInfo) => void;

	constructor(inner: StreamableHTTPClientTransport, sink: MCPMessageSink) {
		this.inner = inner;
		this.#sink = sink;
		inner.onmessage = ((message: JSONRPCMessage, extra?: MessageExtraInfo) => {
			this.#sink?.recordMessage('incoming', message);
			this.onmessage?.(message, extra);
		}) as StreamableHTTPClientTransport['onmessage'];
		inner.onerror = (error) => {
			this.#sink?.recordError(error);
			this.onerror?.(error);
		};
		inner.onclose = () => {
			this.#sink?.recordLifecycle('closed');
			this.onclose?.();
		};
	}

	detach(): void {
		this.#sink = undefined;
	}

	async start(): Promise<void> {
		this.#sink?.recordLifecycle('connecting');
		await this.inner.start();
	}

	async send(message: JSONRPCMessage, options?: TransportSendOptions): Promise<void> {
		this.#sink?.recordMessage('outgoing', message);
		await this.inner.send(message, options);
	}

	close(): Promise<void> {
		return this.inner.close();
	}

	get sessionId(): string | undefined {
		return this.inner.sessionId;
	}

	setProtocolVersion(version: string): void {
		this.inner.setProtocolVersion(version);
	}
}
