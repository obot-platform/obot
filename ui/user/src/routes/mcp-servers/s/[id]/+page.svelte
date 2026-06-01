<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { UserService } from '$lib/services';
	import { getMCPDisplayName } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { type Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { mcpServer, catalogEntry, workspaceId } = $derived(data);
	let title = $derived(getMCPDisplayName(mcpServer) || 'MCP Server');
	let serverWorkspaceId = $derived(mcpServer?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(
		serverWorkspaceId ? ('workspace' as const) : ('catalog' as const)
	);
	let serverScopeID = $derived(
		serverWorkspaceId || mcpServer?.mcpCatalogID || DEFAULT_MCP_CATALOG_ID
	);

	let limitViews = $derived(
		(serverWorkspaceId && workspaceId === serverWorkspaceId) || profile.current.hasAdminAccess?.()
			? ['overview', 'tools', 'server-instances']
			: ['overview', 'tools']
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
		<McpServerActions
			server={mcpServer}
			entry={catalogEntry}
			catalogID={mcpServer?.mcpCatalogID}
			workspaceID={workspaceId}
			allowMultiUserServerConfigurationEdit
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
			<McpServerEntryForm
				entry={mcpServer}
				type="multi"
				id={serverScopeID}
				entity={serverScopeEntity}
				{limitViews}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
