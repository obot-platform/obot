import { m } from '$lib/i18n';
import { MessageSquareText, Server } from '@lucide/svelte';

export const MESSAGE_POLICIES_REDIRECT_DESTINATIONS = [
	{
		kicker: m.ai_judge_title(),
		title: m.ai_judge_mcp_servers(),
		description: m.ai_judge_message_policies_mcp_description(),
		href: '/mcp-servers?view=ai-judge-policies',
		icon: Server
	},
	{
		kicker: m.ai_judge_title(),
		title: m.ai_judge_models(),
		description: m.ai_judge_message_policies_models_description(),
		href: '/models?view=ai-judge-policies',
		icon: MessageSquareText
	}
] as const;
