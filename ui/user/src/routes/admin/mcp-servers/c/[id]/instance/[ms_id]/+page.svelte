<script lang="ts">
	import BackLink from '$lib/components/admin/BackLink.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { Info } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { catalogEntry, mcpServerId } = data;
	const duration = PAGE_TRANSITION_DURATION;

	let catalogEntryName =
		catalogEntry?.urlManifest?.name ?? catalogEntry?.commandManifest?.name ?? 'Unknown';
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServerId}
			{@const currentLabel = mcpServerId ?? 'Server'}
			<BackLink fromURL={`/mcp-servers/${catalogEntry?.id}`} {currentLabel} />
		{/if}

		{#if mcpServerId && catalogEntry?.commandManifest}
			<McpServerK8sInfo {mcpServerId} name={catalogEntryName} />
		{:else}
			<h1 class="text-2xl font-semibold">
				{catalogEntryName} | {mcpServerId}
			</h1>

			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<p>Server information cannot be provided at this time.</p>
				</div>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {catalogEntryName} | {mcpServerId}</title>
</svelte:head>
