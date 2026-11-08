import { doPost } from './http';

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

export async function chatWithAI(messages: AIMessage[]): Promise<AIReply> {
	const resp = (await doPost('/ai/chat', { messages }, { dontLogErrors: true })) as AIReply;
	return { ...resp, steps: resp.steps ?? [] };
}
