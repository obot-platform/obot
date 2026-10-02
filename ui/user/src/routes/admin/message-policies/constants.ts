import { m } from '$lib/i18n';
import { MessageSquareText, Server } from '@lucide/svelte';

export const MESSAGE_POLICIES_REDIRECT_DESTINATIONS = [
	{
		kicker: m.admin_routes_ai_judge_policies(),
		title: m.admin_routes_mcp_servers(),
		description: m.admin_routes_message_policies_mcp_description(),
		href: '/mcp-servers?view=ai-judge-policies',
		icon: Server
	},
	{
		kicker: m.admin_routes_ai_judge_policies(),
		title: m.admin_routes_models(),
		description: m.admin_routes_message_policies_models_description(),
		href: '/models?view=ai-judge-policies',
		icon: MessageSquareText
	}
] as const;
