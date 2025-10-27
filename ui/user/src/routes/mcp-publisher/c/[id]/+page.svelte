<script lang="ts">
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Layout from '$lib/components/Layout.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { workspaceId, catalogEntry: initialCatalogEntry } = data;
	let catalogEntry = $state(initialCatalogEntry);
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	showUserLinks
>
	<div class="flex h-full flex-col gap-6 pt-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if catalogEntry}
			{@const currentLabel = catalogEntry?.manifest?.name ?? 'MCP Server'}
			<BackLink fromURL="mcp-publisher" {currentLabel} />
		{/if}

		{#if workspaceId && catalogEntry}
			<McpServerEntryForm
				entry={catalogEntry}
				type={catalogEntry?.manifest.runtime === 'composite'
					? 'composite'
					: catalogEntry?.manifest.runtime === 'remote'
						? 'remote'
						: 'single'}
				readonly={catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL}
				id={workspaceId}
				entity="workspace"
				onCancel={() => {
					goto('/mcp-publisher');
				}}
				onSubmit={async () => {
					goto('/mcp-publisher');
				}}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {catalogEntry?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
