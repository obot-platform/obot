<script lang="ts">
	import { type Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$lib/url';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { mcpServersAndEntries } from '$lib/stores/index.js';
	import { page } from '$app/state';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { workspaceId, catalogEntry } = $derived(data);
	let title = $derived(catalogEntry?.manifest?.name ?? 'MCP Server');
	const hasExistingConfigured = $derived(
		Boolean(
			catalogEntry &&
				mcpServersAndEntries.current.userConfiguredServers.some(
					(server) => server.catalogEntryID === catalogEntry.id
				)
		)
	);
	let promptInitialLaunch = $derived(page.url.searchParams.get('launch') === 'true');
	let promptOAuthConfig = $derived(page.url.searchParams.get('configure-oauth') === 'true');
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
		<McpServerActions entry={catalogEntry} {promptInitialLaunch} {promptOAuthConfig} />
	{/snippet}
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if catalogEntry}
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
					goto('/mcp-servers');
				}}
				onSubmit={async () => {
					goto('/mcp-servers');
				}}
				{hasExistingConfigured}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
