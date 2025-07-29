<script lang="ts">
	import BackLink from '$lib/components/admin/BackLink.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer, mcpServerInstance } = data;
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			{@const currentLabel = mcpServerInstance?.id ?? 'Server Instance'}
			<BackLink fromURL={`/mcp-servers/${mcpServer.id}`} {currentLabel} />
		{/if}

		{#if mcpServer}
			<McpServerK8sInfo
				mcpServerId={mcpServer.id}
				name={mcpServer.manifest.name || 'Unknown'}
				mcpServerInstanceId={mcpServerInstance?.id}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest?.name} | {mcpServerInstance?.id}</title>
</svelte:head>
