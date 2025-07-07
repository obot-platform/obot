<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import BackLink from '$lib/components/admin/BackLink.svelte';
	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer } = data;
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			{@const currentLabel = mcpServer?.manifest?.name ?? 'MCP Server'}
			<BackLink fromURL={'mcp-servers'} {currentLabel} />
		{/if}

		<McpServerEntryForm
			entry={mcpServer}
			type="multi"
			catalogId={DEFAULT_MCP_CATALOG_ID}
			onCancel={() => {
				goto('/v2/admin/mcp-servers');
			}}
			onSubmit={async () => {
				goto('/v2/admin/mcp-servers');
			}}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
