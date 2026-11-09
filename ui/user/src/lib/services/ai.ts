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

/**
 * A live view Obot AI can place in an answer. It writes one as a fenced `obot-widget` block
 * holding JSON; the page renders the matching component, which loads its data with the viewer's
 * own permissions.
 */
export type AIWidget =
	| { type: 'session'; sessionId: string }
	| { type: 'sessions'; sessionIds: string[] }
	| { type: 'mcp_server_request'; requestId: string }
	| { type: 'approvals' };

export type AISegment =
	| { kind: 'markdown'; text: string }
	| { kind: 'widget'; widget: AIWidget }
	/** A widget the model is still writing, or one that could not be read. */
	| { kind: 'pending' }
	| { kind: 'invalid'; source: string };

const WIDGET_FENCE = '```obot-widget';

/** Splits an answer into markdown and widgets. An unterminated widget block is still being written. */
export function splitAnswer(content: string): AISegment[] {
	const out: AISegment[] = [];
	let rest = content;
	while (rest) {
		const start = rest.indexOf(WIDGET_FENCE);
		if (start < 0) {
			out.push({ kind: 'markdown', text: rest });
			break;
		}
		if (start > 0) out.push({ kind: 'markdown', text: rest.slice(0, start) });
		const body = rest.slice(start + WIDGET_FENCE.length);
		const end = body.indexOf('```');
		if (end < 0) {
			out.push({ kind: 'pending' });
			break;
		}
		out.push(parseWidget(body.slice(0, end)));
		rest = body.slice(end + 3);
	}
	return out.filter((s) => s.kind !== 'markdown' || s.text.trim() !== '');
}

function parseWidget(source: string): AISegment {
	try {
		const w = JSON.parse(source.trim()) as Record<string, unknown>;
		const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
		switch (w.type) {
			case 'session':
			case 'session_timeline': {
				const sessionId = str(w.sessionId);
				if (sessionId) return { kind: 'widget', widget: { type: 'session', sessionId } };
				break;
			}
			case 'sessions': {
				const ids = Array.isArray(w.sessionIds) ? w.sessionIds.map(str).filter((id) => !!id) : [];
				if (ids.length)
					return {
						kind: 'widget',
						widget: { type: 'sessions', sessionIds: (ids as string[]).slice(0, 20) }
					};
				break;
			}
			case 'approvals':
				return { kind: 'widget', widget: { type: 'approvals' } };
			case 'mcp_server_request': {
				const requestId = str(w.requestId);
				if (requestId) return { kind: 'widget', widget: { type: 'mcp_server_request', requestId } };
				break;
			}
		}
	} catch {
		// Falls through to invalid.
	}
	return { kind: 'invalid', source: source.trim() };
}
