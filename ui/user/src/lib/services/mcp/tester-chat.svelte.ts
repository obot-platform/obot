import type {
	DirectOperationResult,
	MCPTesterSession,
	StagedTesterContext,
	TesterWorkflow
} from './tester.svelte';
import type { CallToolResult, Tool } from '@modelcontextprotocol/sdk/types.js';

export const MCP_TESTER_MAX_MODEL_BYTES = 100 * 1024;
export const MCP_TESTER_MAX_ROUNDS = 10;

export type TesterChatRole = 'user' | 'assistant' | 'tool';
export type TesterChatContentType = 'text' | 'resource';
export type TesterToolResultStatus = 'success' | 'error' | 'rejected' | 'cancelled';
export type TesterChatTurnStatus =
	| 'idle'
	| 'snapshotting'
	| 'streaming'
	| 'approval'
	| 'executing-tools'
	| 'failed'
	| 'cancelled'
	| 'round-limit';

export interface TesterChatContent {
	type: TesterChatContentType;
	text: string;
	uri?: string;
	mimeType?: string;
}

export interface TesterChatTool {
	name: string;
	description?: string;
	inputSchema: Record<string, unknown>;
	outputSchema?: Record<string, unknown>;
}

export interface TesterChatToolCall {
	id: string;
	name: string;
	arguments: Record<string, unknown>;
}

export interface TesterChatToolResult {
	callID: string;
	status: TesterToolResultStatus;
	content: unknown;
}

export interface TesterChatMessage {
	role: TesterChatRole;
	content?: TesterChatContent[];
	toolCalls?: TesterChatToolCall[];
	toolResult?: TesterChatToolResult;
}

export interface TesterChatError {
	code: string;
	message: string;
	retryable?: boolean;
}

export interface TesterToolApproval extends TesterChatToolCall {
	decision: 'pending' | 'approved' | 'rejected';
	execution: 'waiting' | 'executing' | 'complete';
	result?: DirectOperationResult<CallToolResult>;
	modelResult?: TesterChatToolResult;
}

export interface TesterChatTimelineMessage {
	id: string;
	role: TesterChatRole;
	content?: TesterChatContent[];
	text?: string;
	state?: 'streaming' | 'complete' | 'failed' | 'cancelled';
	error?: TesterChatError;
	toolCalls?: TesterToolApproval[];
	stagedName?: string;
}

interface TesterStreamEvent {
	type: 'assistant_message_start' | 'text_delta' | 'tool_calls' | 'completion' | 'error';
	delta?: string;
	calls?: TesterChatToolCall[];
	reason?: 'stop' | 'tool_calls' | 'max_tokens';
	error?: TesterChatError;
}

interface StreamAttempt {
	timelineID: string;
	round: number;
}

function randomID(): string {
	return crypto.randomUUID();
}

function byteLength(value: string): number {
	return new TextEncoder().encode(value).byteLength;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function isAbortError(error: unknown): boolean {
	return (
		(error instanceof DOMException && error.name === 'AbortError') ||
		(error instanceof Error && error.name === 'AbortError')
	);
}

function normalizedTool(tool: Tool): TesterChatTool {
	return {
		name: tool.name,
		description: tool.description,
		inputSchema: tool.inputSchema,
		outputSchema: tool.outputSchema
	};
}

function stagedMessages(contexts: StagedTesterContext[]): {
	messages: TesterChatMessage[];
	timeline: TesterChatTimelineMessage[];
} {
	const messages: TesterChatMessage[] = [];
	const timeline: TesterChatTimelineMessage[] = [];

	for (const context of contexts) {
		if (context.type === 'prompt') {
			for (const message of context.messages) {
				if (message.content.type !== 'text') continue;
				const content: TesterChatContent[] = [{ type: 'text', text: message.content.text }];
				messages.push({ role: message.role, content });
				timeline.push({
					id: randomID(),
					role: message.role,
					content,
					text: message.content.text,
					state: 'complete',
					stagedName: context.name
				});
			}
			continue;
		}

		const content: TesterChatContent[] = context.contents.map((item) => ({
			type: 'resource',
			text: item.text,
			uri: item.uri,
			mimeType: item.mimeType
		}));
		messages.push({ role: 'user', content });
		timeline.push({
			id: randomID(),
			role: 'user',
			content,
			state: 'complete',
			stagedName: context.name
		});
	}

	return { messages, timeline };
}

function parseToolCall(value: unknown): TesterChatToolCall {
	if (!value || typeof value !== 'object')
		throw new Error('The model returned an invalid tool call');
	const call = value as { id?: unknown; name?: unknown; arguments?: unknown };
	if (typeof call.id !== 'string' || typeof call.name !== 'string') {
		throw new Error('The model returned a tool call without an ID or name');
	}
	if (!call.arguments || typeof call.arguments !== 'object' || Array.isArray(call.arguments)) {
		throw new Error(`The model returned invalid arguments for ${call.name}`);
	}
	return { id: call.id, name: call.name, arguments: call.arguments as Record<string, unknown> };
}

function parseStreamEvent(value: unknown): TesterStreamEvent {
	if (
		!value ||
		typeof value !== 'object' ||
		typeof (value as { type?: unknown }).type !== 'string'
	) {
		throw new Error('The model returned an invalid stream event');
	}
	const event = value as TesterStreamEvent;
	if (event.type === 'tool_calls') {
		event.calls = (event.calls ?? []).map(parseToolCall);
	}
	return event;
}

async function readEventStream(
	response: Response,
	onEvent: (event: TesterStreamEvent) => void
): Promise<void> {
	if (!response.body) throw new Error('The model response did not contain a stream');
	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffered = '';
	let dataLines: string[] = [];

	const consumeLine = (line: string) => {
		if (line === '') {
			if (dataLines.length) {
				let value: unknown;
				try {
					value = JSON.parse(dataLines.join('\n'));
				} catch {
					throw {
						code: 'unsupported_response',
						message: 'The model returned malformed streaming JSON',
						retryable: false
					} satisfies TesterChatError;
				}
				onEvent(parseStreamEvent(value));
				dataLines = [];
			}
			return;
		}
		if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
	};

	while (true) {
		const { done, value } = await reader.read();
		buffered += decoder.decode(value, { stream: !done });
		const lines = buffered.split(/\r?\n/);
		buffered = done ? '' : (lines.pop() ?? '');
		for (const line of lines) consumeLine(line);
		if (done) break;
	}
	if (buffered) consumeLine(buffered);
	consumeLine('');
}

async function responseError(response: Response): Promise<TesterChatError> {
	try {
		const value = (await response.json()) as { error?: TesterChatError };
		if (value.error?.message) return value.error;
	} catch {
		// Fall through to the stable transport error below.
	}
	return {
		code: 'provider_error',
		message: `Chat request failed with status ${response.status}`,
		retryable: response.status >= 500
	};
}

function modelBoundResult(status: TesterToolResultStatus, content: unknown): unknown {
	const serialized = JSON.stringify({ status, content });
	if (byteLength(serialized) <= MCP_TESTER_MAX_MODEL_BYTES) return content;
	return {
		oversized: true,
		message:
			status === 'success'
				? 'The tool succeeded, but its output was too large to include.'
				: 'The tool result was too large to include.'
	};
}

export class MCPTesterChat {
	readonly #session: MCPTesterSession;
	readonly #serverID: string;
	readonly #fetcher: typeof fetch;
	#workflow?: TesterWorkflow;
	#toolAbort?: AbortController;
	#generation = 0;
	#retryAttempt?: StreamAttempt;
	#resolvingBatch = false;

	messages = $state<TesterChatMessage[]>([]);
	timeline = $state<TesterChatTimelineMessage[]>([]);
	toolSnapshot = $state<TesterChatTool[]>();
	status = $state<TesterChatTurnStatus>('idle');
	error = $state<TesterChatError>();
	round = $state(0);

	constructor(session: MCPTesterSession, serverID: string, fetcher: typeof fetch = fetch) {
		this.#session = session;
		this.#serverID = serverID;
		this.#fetcher = fetcher;
	}

	get hasConversation(): boolean {
		return this.timeline.length > 0;
	}

	get approvalNeeded(): boolean {
		return Boolean(this.activeApproval);
	}

	get pendingApprovals(): TesterToolApproval[] | undefined {
		return this.timeline.findLast((message) => message.toolCalls)?.toolCalls;
	}

	get activeApproval(): TesterToolApproval | undefined {
		if (this.status !== 'approval') return undefined;
		return this.pendingApprovals?.find((call) => call.decision === 'pending');
	}

	get executingApproval(): TesterToolApproval | undefined {
		return this.pendingApprovals?.find((call) => call.execution === 'executing');
	}

	get canSend(): boolean {
		return !this.#session.activeWorkflow && this.#session.status === 'ready';
	}

	async send(text: string): Promise<void> {
		if (!this.canSend) return;
		const contexts = [...this.#session.stagedContext];
		if (!text.trim() && contexts.length === 0) return;

		const workflow = this.#session.beginWorkflow('chat', 'chat turn');
		this.#workflow = workflow;
		this.#retryAttempt = undefined;
		this.error = undefined;
		this.round = 1;
		const generation = this.#generation;

		try {
			if (this.toolSnapshot === undefined) {
				this.status = 'snapshotting';
				const tools = await this.#session.snapshotChatTools(workflow.abort.signal);
				if (!this.#isCurrent(workflow, generation)) return;
				this.toolSnapshot = tools.map(normalizedTool);
			}

			const staged = stagedMessages(contexts);
			this.messages.push(...staged.messages);
			this.timeline.push(...staged.timeline);
			if (text.trim()) {
				const content: TesterChatContent[] = [{ type: 'text', text }];
				this.messages.push({ role: 'user', content });
				this.timeline.push({
					id: randomID(),
					role: 'user',
					content,
					text,
					state: 'complete'
				});
			}
			for (const context of contexts) this.#session.removeStagedContext(context.id);
			await this.#stream(workflow, { timelineID: randomID(), round: 1 }, generation);
		} catch (error) {
			this.#failBeforeStream(error, workflow, generation);
		}
	}

	stop(): void {
		if (this.status !== 'snapshotting' && this.status !== 'streaming') return;
		const attempt = this.#currentAssistant();
		if (attempt) attempt.state = 'cancelled';
		this.status = 'cancelled';
		this.error = { code: 'cancelled', message: 'Stopped by user' };
		this.#workflow?.abort.abort('Stopped by user');
		this.#finishWorkflow();
	}

	async retry(): Promise<void> {
		if (!this.canSend || !this.#retryAttempt) return;
		const workflow = this.#session.beginWorkflow('chat', 'chat retry');
		this.#workflow = workflow;
		this.error = undefined;
		const generation = this.#generation;
		await this.#stream(workflow, this.#retryAttempt, generation);
	}

	approve(callID: string): void {
		this.#decide(callID, 'approved');
	}

	reject(callID: string): void {
		this.#decide(callID, 'rejected');
	}

	cancelExecutingTool(): void {
		this.#toolAbort?.abort('Cancelled by user');
	}

	newChat(): void {
		this.#generation += 1;
		this.#toolAbort?.abort('New Chat');
		this.#workflow?.abort.abort('New Chat');
		this.#finishWorkflow();
		this.messages = [];
		this.timeline = [];
		this.toolSnapshot = undefined;
		this.status = 'idle';
		this.error = undefined;
		this.round = 0;
		this.#retryAttempt = undefined;
		this.#resolvingBatch = false;
		this.#session.stagedContext = [];
	}

	close(): void {
		this.newChat();
	}

	#decide(callID: string, decision: 'approved' | 'rejected'): void {
		if (this.status !== 'approval') return;
		const call = this.pendingApprovals?.find((item) => item.id === callID);
		if (!call || call.decision !== 'pending') return;
		call.decision = decision;
		// The queue runs as far as it can and parks on the next undecided call,
		// so an approved tool executes without waiting on its siblings.
		void this.#resolveBatch();
	}

	async #executeCall(
		call: TesterToolApproval,
		workflow: TesterWorkflow,
		generation: number
	): Promise<void> {
		let modelResult: TesterChatToolResult;

		if (call.decision === 'rejected') {
			modelResult = {
				callID: call.id,
				status: 'rejected',
				content: 'rejected by user'
			};
		} else {
			call.execution = 'executing';
			const controller = new AbortController();
			this.#toolAbort = controller;
			const abortTool = () => controller.abort(workflow.abort.signal.reason);
			workflow.abort.signal.addEventListener('abort', abortTool, { once: true });
			const rawResult = await this.#session.callChatTool(
				call.name,
				call.arguments,
				controller.signal
			);
			workflow.abort.signal.removeEventListener('abort', abortTool);
			this.#toolAbort = undefined;
			const status: TesterToolResultStatus =
				rawResult.status === 'success'
					? 'success'
					: rawResult.status === 'cancelled'
						? 'cancelled'
						: 'error';
			const content = rawResult.value ?? {
				category: rawResult.status,
				message: rawResult.message || 'The tool call failed'
			};
			modelResult = {
				callID: call.id,
				status,
				content: modelBoundResult(status, content)
			};
			call.result = rawResult;
		}

		// The "New Chat" button can clear the conversation while a tool is executing.
		// So if the chat generation doesn't match, don't show the tool result.
		if (this.#generation !== generation) return;

		call.execution = 'complete';
		call.modelResult = modelResult;
		this.messages.push({ role: 'tool', toolResult: modelResult });
	}

	async #resolveBatch(): Promise<void> {
		if (this.#resolvingBatch || !this.#workflow) return;
		const calls = this.pendingApprovals;
		if (!calls?.length) return;
		this.#resolvingBatch = true;
		const workflow = this.#workflow;
		const generation = this.#generation;

		try {
			for (const call of calls) {
				if (call.execution === 'complete') continue;
				if (call.decision === 'pending') {
					// Park here until the user decides this one.
					this.status = 'approval';
					return;
				}
				if (!this.#isCurrent(workflow, generation)) return;
				this.status = 'executing-tools';
				await this.#executeCall(call, workflow, generation);
			}
		} finally {
			this.#resolvingBatch = false;
		}

		if (!this.#isCurrent(workflow, generation)) return;
		if (this.round >= MCP_TESTER_MAX_ROUNDS) {
			this.status = 'round-limit';
			this.error = {
				code: 'round_limit',
				message: `This turn reached the ${MCP_TESTER_MAX_ROUNDS}-round limit. Send Continue to start a new turn.`
			};
			this.#finishWorkflow();
			return;
		}

		this.round += 1;
		await this.#stream(workflow, { timelineID: randomID(), round: this.round }, generation);
	}

	async #stream(
		workflow: TesterWorkflow,
		attempt: StreamAttempt,
		generation: number
	): Promise<void> {
		this.status = 'streaming';
		this.round = attempt.round;
		this.#retryAttempt = attempt;
		let assistant = this.timeline.find((message) => message.id === attempt.timelineID);
		if (assistant) {
			assistant.text = '';
			assistant.content = [];
			assistant.state = 'streaming';
			assistant.error = undefined;
			assistant.toolCalls = undefined;
		} else {
			this.timeline.push({
				id: attempt.timelineID,
				role: 'assistant',
				text: '',
				content: [],
				state: 'streaming'
			});
			// Mutate the deep-state proxy, rather than the plain object that was inserted.
			assistant = this.timeline.at(-1)!;
		}

		try {
			const response = await this.#fetcher(
				`/api/mcp-servers/${encodeURIComponent(this.#serverID)}/tester/chat`,
				{
					method: 'POST',
					credentials: 'same-origin',
					headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
					body: JSON.stringify({
						messages: this.messages,
						tools: this.toolSnapshot ?? [],
						round: attempt.round
					}),
					signal: workflow.abort.signal
				}
			);
			if (!response.ok) throw await responseError(response);

			await readEventStream(response, (event) => {
				if (!this.#isCurrent(workflow, generation)) return;
				this.#consumeEvent(assistant, event);
			});
			if (assistant.state === 'streaming') {
				throw new Error('The model stream ended without a completion event');
			}
		} catch (error) {
			if (!this.#isCurrent(workflow, generation)) return;
			if (workflow.abort.signal.aborted || isAbortError(error)) {
				assistant.state = 'cancelled';
				this.status = 'cancelled';
				this.error = { code: 'cancelled', message: 'Stopped by user' };
			} else {
				const normalized =
					error && typeof error === 'object' && 'code' in error && 'message' in error
						? (error as TesterChatError)
						: { code: 'provider_error', message: errorMessage(error), retryable: true };
				assistant.state = 'failed';
				assistant.error = normalized;
				this.status = 'failed';
				this.error = normalized;
				if (normalized.code === 'access_denied') this.#session.status = 'access-denied';
			}
			this.#finishWorkflow();
		}
	}

	#consumeEvent(assistant: TesterChatTimelineMessage, event: TesterStreamEvent): void {
		switch (event.type) {
			case 'assistant_message_start':
				return;
			case 'text_delta':
				assistant.text = (assistant.text ?? '') + (event.delta ?? '');
				assistant.content = assistant.text ? [{ type: 'text', text: assistant.text }] : [];
				return;
			case 'tool_calls':
				assistant.toolCalls = (event.calls ?? []).map((call) => ({
					...call,
					decision: 'pending',
					execution: 'waiting'
				}));
				return;
			case 'completion': {
				if (!assistant.text && !assistant.toolCalls?.length) {
					throw {
						code: 'unsupported_response',
						message: 'The model returned an empty assistant response',
						retryable: false
					} satisfies TesterChatError;
				}
				assistant.state = 'complete';
				const message: TesterChatMessage = { role: 'assistant' };
				if (assistant.text) message.content = [{ type: 'text', text: assistant.text }];
				if (assistant.toolCalls?.length) {
					message.toolCalls = assistant.toolCalls.map(({ id, name, arguments: args }) => ({
						id,
						name,
						arguments: args
					}));
				}
				this.messages.push(message);
				this.#retryAttempt = undefined;
				if (event.reason === 'tool_calls' && assistant.toolCalls?.length) {
					this.status = 'approval';
				} else {
					if (event.reason === 'max_tokens') {
						this.error = {
							code: 'max_tokens',
							message: 'The model stopped because it reached its output limit.'
						};
					}
					this.status = 'idle';
					this.#finishWorkflow();
				}
				return;
			}
			case 'error':
				throw (
					event.error ?? {
						code: 'unsupported_response',
						message: 'The model returned an unsupported response'
					}
				);
		}
	}

	#failBeforeStream(error: unknown, workflow: TesterWorkflow, generation: number): void {
		if (!this.#isCurrent(workflow, generation)) return;
		if (workflow.abort.signal.aborted || isAbortError(error)) {
			this.status = 'cancelled';
			this.error = { code: 'cancelled', message: 'Stopped by user' };
		} else {
			this.status = 'failed';
			this.error = { code: 'transport_error', message: errorMessage(error), retryable: true };
		}
		this.#finishWorkflow();
	}

	#currentAssistant(): TesterChatTimelineMessage | undefined {
		return this.timeline.findLast((message) => message.role === 'assistant');
	}

	#isCurrent(workflow: TesterWorkflow, generation: number): boolean {
		return this.#workflow?.id === workflow.id && this.#generation === generation;
	}

	#finishWorkflow(): void {
		if (this.#workflow) this.#session.finishWorkflow(this.#workflow);
		this.#workflow = undefined;
	}
}
