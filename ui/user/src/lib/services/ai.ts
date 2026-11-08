import { baseURL, doPost } from './http';

// Obot AI: a chat that answers questions about Obot and acts with the signed-in user's
// permissions, using the tools of Obot's built-in MCP servers.

export interface AIMessage {
	role: 'user' | 'assistant';
	content: string;
}

export interface AIStep {
	tool: string;
	arguments?: Record<string, unknown>;
	isError?: boolean;
	result?: string;
}

export interface AIReply {
	reply: string;
	model?: string;
	steps: AIStep[];
}

/** A server-sent event of a streamed answer. */
export type AIEvent =
	| { type: 'thinking' }
	| { type: 'tool_start'; tool: string; arguments?: Record<string, unknown> }
	| { type: 'tool_end'; tool: string; isError?: boolean }
	| { type: 'delta'; text: string }
	| { type: 'done'; reply: string; model?: string; steps?: AIStep[] }
	| { type: 'error'; message: string };

export async function chatWithAI(messages: AIMessage[]): Promise<AIReply> {
	const resp = (await doPost('/ai/chat', { messages }, { dontLogErrors: true })) as AIReply;
	return { ...resp, steps: resp.steps ?? [] };
}

/**
 * Streams an answer: onEvent receives each tool call as it starts and ends and the answer's
 * text as it is written. Resolves with the complete reply.
 */
export async function streamChatWithAI(
	messages: AIMessage[],
	onEvent: (event: AIEvent) => void,
	signal?: AbortSignal
): Promise<AIReply> {
	const response = await fetch(`${baseURL}/ai/chat`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
		body: JSON.stringify({ messages }),
		signal
	});
	if (!response.ok) {
		const text = await response.text();
		throw new Error(text.trim() || response.statusText);
	}
	if (!response.headers.get('Content-Type')?.includes('text/event-stream') || !response.body) {
		const reply = (await response.json()) as AIReply;
		return { ...reply, steps: reply.steps ?? [] };
	}

	let result: AIReply | undefined;
	let failure: string | undefined;
	const handle = (event: AIEvent) => {
		if (event.type === 'done') {
			result = { reply: event.reply ?? '', model: event.model, steps: event.steps ?? [] };
		} else if (event.type === 'error') {
			failure = event.message;
		}
		onEvent(event);
	};

	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffered = '';
	let data: string[] = [];
	const consume = (line: string) => {
		if (line === '') {
			if (data.length) {
				try {
					handle(JSON.parse(data.join('\n')) as AIEvent);
				} catch {
					// Ignore a malformed event rather than lose the rest of the answer.
				}
				data = [];
			}
			return;
		}
		if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
	};
	while (true) {
		const { done, value } = await reader.read();
		buffered += decoder.decode(value, { stream: !done });
		const lines = buffered.split(/\r?\n/);
		buffered = done ? '' : (lines.pop() ?? '');
		for (const line of lines) consume(line);
		if (done) break;
	}
	consume('');
	if (failure) throw new Error(failure);
	if (!result) throw new Error('the answer ended early');
	return result;
}
