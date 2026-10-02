<script lang="ts">
	import RedirectLayout from '$lib/components/RedirectLayout.svelte';
	import { m } from '$lib/i18n';
	import { MessageSquareText, Server } from '@lucide/svelte';

	const destinations = [
		{
			kicker: m.admin_routes_policy_violations(),
			title: m.admin_routes_mcp_servers(),
			description: m.admin_routes_policy_violations_mcp_description(),
			href: '/mcp-servers?view=ai-judge-policies&contents=policy-violations',
			icon: Server
		},
		{
			kicker: m.admin_routes_policy_violations(),
			title: m.admin_routes_models(),
			description: m.admin_routes_policy_violations_models_description(),
			href: '/models?view=ai-judge-policies&contents=policy-violations',
			icon: MessageSquareText
		}
	] as const;
</script>

<RedirectLayout title={m.admin_routes_message_policy_violations()} {destinations} />
