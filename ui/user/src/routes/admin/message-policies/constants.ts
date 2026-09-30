import { MessageSquareText, Server } from '@lucide/svelte';

export const MESSAGE_POLICIES_REDIRECT_DESTINATIONS = [
	{
		kicker: 'AI Judge Policies',
		title: 'MCP Servers',
		description: 'Enforce MCP server tool calls with the LLM.',
		href: '/mcp-servers?view=ai-judge-policies',
		icon: Server
	},
	{
		kicker: 'AI Judge Policies',
		title: 'Models',
		description: 'Enforce user messages with the LLM.',
		href: '/models?view=ai-judge-policies',
		icon: MessageSquareText
	}
] as const;
