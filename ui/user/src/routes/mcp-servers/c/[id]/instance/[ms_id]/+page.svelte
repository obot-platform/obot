<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { getMCPDisplayName, isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { CircleAlert } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { workspaceId, catalogEntry, mcpServer } = $derived(data);
	let title = $derived(
		getMCPDisplayName(mcpServer) || getMCPDisplayName(catalogEntry) || 'MCP Server'
	);
	let deprecated = $derived(
		isDeprecatedMCPServer(catalogEntry) || isDeprecatedMCPServer(mcpServer)
	);
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	{title}
	showBackButton
>
	{#snippet rightNavActions()}
		<McpServerActions entry={catalogEntry} server={mcpServer} />
	{/snippet}
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if deprecated}
			<div class="border-warning bg-warning/10 flex items-start gap-3 rounded-lg border p-4">
				<CircleAlert class="text-warning mt-0.5 size-5 shrink-0" />
				<div class="flex-1">
					<p class="text-sm font-medium">This server is deprecated.</p>
					<p class="text-muted-content mt-1 text-xs">
						It may stop receiving updates or be removed in a future catalog release. Use a
						replacement server when possible.
					</p>
				</div>
			</div>
		{/if}

		{#if catalogEntry}
			<McpServerEntryForm
				entry={catalogEntry}
				server={mcpServer}
				type={catalogEntry?.manifest.runtime === 'composite'
					? 'composite'
					: catalogEntry?.manifest.runtime === 'remote'
						? 'remote'
						: 'hosted'}
				readonly={catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL}
				id={workspaceId}
				entity="workspace"
				connectOnly
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
