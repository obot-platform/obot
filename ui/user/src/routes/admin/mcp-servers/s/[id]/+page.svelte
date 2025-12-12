<script lang="ts">
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$lib/url';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { profile } from '$lib/stores/index.js';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { page } from '$app/state';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer } = $derived(data);
	let title = $derived(mcpServer?.manifest?.name ?? 'MCP Server');
	let promptInitialLaunch = $derived(page.url.searchParams.get('launch') === 'true');
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
		<McpServerActions server={mcpServer} {promptInitialLaunch} />
	{/snippet}

	<div class="flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
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
	<title>Obot | {title}</title>
</svelte:head>
