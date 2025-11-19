<script lang="ts">
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { goto } from '$app/navigation';
	import { profile } from '$lib/stores/index.js';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer: initialMcpServer, workspaceId } = data;
	let mcpServer = $state(initialMcpServer);
	let title = $derived(mcpServer?.manifest?.name ?? 'MCP Server');
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	{title}
	showBackButton
>
	<div class="flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if workspaceId && mcpServer}
			<McpServerEntryForm
				entry={mcpServer}
				type="multi"
				id={workspaceId}
				entity="workspace"
				onCancel={() => {
					goto('/admin/mcp-servers');
				}}
				onSubmit={async () => {
					goto('/admin/mcp-servers');
				}}
				readonly={profile.current.isAdminReadonly?.()}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
