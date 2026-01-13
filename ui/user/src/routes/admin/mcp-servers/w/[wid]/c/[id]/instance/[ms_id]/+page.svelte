<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { profile } from '$lib/stores/index.js';
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { workspaceId, catalogEntry, mcpServer, belongsToUser } = $derived(data);
	let title = $derived(catalogEntry?.manifest?.name ?? 'MCP Server');

	function navigateToMcpServers() {
		goto(resolve(`/admin/mcp-servers`));
	}
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
		{#if workspaceId && catalogEntry}
			<McpServerEntryForm
				entry={catalogEntry}
				server={mcpServer}
				type={catalogEntry?.manifest.runtime === 'remote' ? 'remote' : 'single'}
				id={workspaceId}
				entity="workspace"
				onCancel={navigateToMcpServers}
				onSubmit={navigateToMcpServers}
				readonly={belongsToUser ? false : profile.current.isAdminReadonly?.()}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
