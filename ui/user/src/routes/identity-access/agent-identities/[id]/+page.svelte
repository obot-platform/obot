<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import AgentIdentityDetails from '$lib/components/agent-identities/AgentIdentityDetails.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { agent, keys, creatorName, readonly } = $derived(data);
	const title = $derived(agent?.displayName || m.identity_access_agents_agent());
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if agent}
			{#key agent.id}
				<AgentIdentityDetails
					{agent}
					{keys}
					{creatorName}
					{readonly}
					onDeleted={() => goto('/identity-access?view=agent-identities', { replaceState: true })}
				/>
			{/key}
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
