import { MessageSquareText, Server } from '@lucide/svelte';

export const MESSAGE_POLICIES_REDIRECT_DESTINATIONS = [
	{
		kicker: 'Message Policies',
		title: 'MCP Servers',
		description: 'Set up message policies on server tool calls.',
		href: '/mcp-servers?view=message-policies',
		icon: Server
	},
	{
		kicker: 'Message Policies',
		title: 'Models',
		description: 'Set up message policies when sending messages to the LLM.',
		href: '/models?view=message-policies',
		icon: MessageSquareText
	}
] as const;
