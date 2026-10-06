<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import HostedAgentForm from '$lib/components/admin/HostedAgentForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { hostedAgent } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(hostedAgent?.name ?? m.hosted_agents_agent_template());
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<HostedAgentForm
			{hostedAgent}
			onUpdate={() => {
				goto('/hosted-agents');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>{m.chat_page_title_named({ name: title })}</title>
</svelte:head>
