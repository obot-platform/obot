<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import BackLink from '$lib/components/admin/BackLink.svelte';
	import { AdminService } from '$lib/services/index.js';
	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer: initialMcpServer } = data;
	let mcpServer = $state(initialMcpServer);
</script>

<Layout>
	<div class="mt-6 flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			{@const currentLabel = mcpServer?.manifest?.name ?? 'MCP Server'}
			<BackLink fromURL="mcp-servers" {currentLabel} />
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
			onUpdate={async () => {
				if (!mcpServer) return;
				mcpServer = await AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, mcpServer.id);
			}}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
