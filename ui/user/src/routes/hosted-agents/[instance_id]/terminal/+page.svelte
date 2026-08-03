<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import AgentTerminal from '$lib/components/hosted-agents/AgentTerminal.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	const duration = PAGE_TRANSITION_DURATION;
	// A sandbox has a console only while it is running, and only if the agent was
	// given one. Both are worth saying plainly rather than letting the attach
	// fail on a blank screen.
	const ready = $derived(!data.instance.deleted && data.instance.status?.state === 'ready');
</script>

<Layout
	title={data.instance.name}
	subtitle="Terminal"
	showBackButton
	onBackButtonClick={() => goto(resolve('/hosted-agents'))}
	alwaysShowHeaderTitle
	classes={{
		container: 'p-0 md:px-8 md:pb-4',
		childrenContainer: 'min-h-0'
	}}
>
	<!-- The terminal fills whatever the nav leaves, so min-h-0 has to run the
	     whole way down: a flex child will not shrink below its content otherwise
	     and the console would push the page into a scrollbar. -->
	<div
		class="flex min-h-0 grow flex-col"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !data.agent.terminal}
			<p class="notification-error text-sm" role="alert">
				{data.agent.name} does not offer a terminal.
			</p>
		{:else if !ready}
			<p class="notification-info text-sm">
				This instance is not running yet, so there is no console to attach to.
				{data.instance.status?.message ?? data.instance.status?.error ?? ''}
			</p>
		{:else}
			<AgentTerminal instanceID={data.instance.id} />
		{/if}
	</div>
</Layout>
