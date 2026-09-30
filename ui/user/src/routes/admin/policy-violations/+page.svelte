<script lang="ts">
	import RedirectLayout from '$lib/components/RedirectLayout.svelte';
	import { MessageSquareText, Server } from '@lucide/svelte';

	const destinations = [
		{
			kicker: 'Policy Violations',
			title: 'MCP Servers',
			description: 'Review policy violations on MCPserver tool calls.',
			href: '/mcp-servers?view=ai-judge-policies&contents=policy-violations',
			icon: Server
		},
		{
			kicker: 'Policy Violations',
			title: 'Models',
			description: 'Review policy violations on user-sent messages to the LLM.',
			href: '/models?view=ai-judge-policies&contents=policy-violations',
			icon: MessageSquareText
		}
	] as const;
</script>

<RedirectLayout title="Message Policy Violations" {destinations} />
