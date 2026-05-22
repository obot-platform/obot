<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { UserService } from '$lib/services';
	import { type Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer, catalogEntry, workspaceId } = $derived(data);
	let title = $derived(mcpServer?.alias || mcpServer?.manifest?.name || 'MCP Server');
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
		<McpServerActions
			server={mcpServer}
			entry={catalogEntry}
			catalogID={mcpServer?.mcpCatalogID}
			workspaceID={workspaceId}
			allowMultiUserServerConfigurationEdit
			{promptInitialLaunch}
			onOAuthConfigured={() => {
				if (!mcpServer) return;
				UserService.getMcpCatalogServer(mcpServer.id).then((server) => {
					mcpServer = server;
				});
			}}
		/>
	{/snippet}
	<div class="flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			<McpServerEntryForm entry={mcpServer} type="multi" id={workspaceId} entity="workspace" />
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
