<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { type Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer, workspaceId, belongsToUser } = $derived(data);
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
	{#snippet rightNavActions()}
		<McpServerActions server={mcpServer} />
	{/snippet}
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
				readonly={belongsToUser ? false : profile.current.isAdminReadonly?.()}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
