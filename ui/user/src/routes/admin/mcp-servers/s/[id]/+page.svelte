<script lang="ts">
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { profile } from '$lib/stores/index.js';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer: initialMcpServer } = data;
	let mcpServer = $state(initialMcpServer);
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
>
	<div class="mt-6 flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			{@const currentLabel = mcpServer?.manifest?.name ?? 'MCP Server'}
			<BackLink fromURL="mcp-servers" {currentLabel} />
		{/if}

		<McpServerEntryForm
			entry={mcpServer}
			type="multi"
			id={DEFAULT_MCP_CATALOG_ID}
			onCancel={() => {
				goto('/admin/mcp-servers');
			}}
			onSubmit={async () => {
				goto('/admin/mcp-servers');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
