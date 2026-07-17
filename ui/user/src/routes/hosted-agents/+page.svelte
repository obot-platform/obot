<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import HostedAgentCard from '$lib/components/hosted-agents/HostedAgentCard.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { Bot } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let hostedAgents = $state(untrack(() => data.hostedAgents));

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout title="Agents">
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if hostedAgents.length === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Bot class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No agents available</h4>
				<p class="text-muted-content text-sm font-light">
					You don't have access to any agents yet. Contact your administrator to request access.
				</p>
			</div>
		{:else}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each hostedAgents as agent (agent.id)}
					<HostedAgentCard {agent} />
				{/each}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
