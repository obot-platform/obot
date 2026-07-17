<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import HostedAgentForm from '$lib/components/admin/HostedAgentForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { hostedAgent } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(hostedAgent?.name ?? 'Agent');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if hostedAgent?.status?.url}
			<div class="mb-4 flex items-center gap-2 text-sm">
				<span class="text-muted-content font-light">URL:</span>
				<a
					href={hostedAgent.status.url}
					target="_blank"
					rel="external noopener noreferrer"
					class="link"
				>
					{hostedAgent.status.url}
				</a>
			</div>
		{/if}
		<HostedAgentForm
			{hostedAgent}
			onUpdate={() => {
				goto('/admin/hosted-agents');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
