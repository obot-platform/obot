<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import BackLink from '$lib/components/admin/BackLink.svelte';
	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { catalogEntry: initialCatalogEntry } = data;
	let catalogEntry = $state(initialCatalogEntry);
</script>

<Layout>
	<div class="mt-6 flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if catalogEntry}
			{@const currentLabel = catalogEntry?.manifest?.name ?? 'MCP Server'}
			<BackLink fromURL="mcp-servers" {currentLabel} />
		{/if}

		<McpServerEntryForm
			entry={catalogEntry}
			type={catalogEntry?.manifest.runtime === 'remote' ? 'remote' : 'single'}
			readonly={catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL}
			catalogId={DEFAULT_MCP_CATALOG_ID}
			onCancel={() => {
				goto('/admin/mcp-servers');
			}}
			onSubmit={async () => {
				goto('/admin/mcp-servers');
			}}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {catalogEntry?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
