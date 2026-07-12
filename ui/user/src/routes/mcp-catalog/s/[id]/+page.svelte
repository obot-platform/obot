<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import { getMCPDisplayName, isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import McpConnectUrlDialog from '../../McpConnectUrlDialog.svelte';
	import { Link2Icon } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let connectUrlDialog = $state<ReturnType<typeof McpConnectUrlDialog>>();

	let { data } = $props();
	let { mcpServer, catalogEntry } = $derived(data);
	let workspaceId = $derived(mcpServer?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(workspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(workspaceId || mcpServer?.mcpCatalogID || DEFAULT_MCP_CATALOG_ID);
	let title = $derived(getMCPDisplayName(mcpServer) || 'MCP Server');
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
		<McpServerActions
			server={mcpServer}
			entry={catalogEntry}
			catalogID={workspaceId ? undefined : serverScopeID}
			workspaceID={workspaceId}
			readonly={profile.current.isAdminReadonly?.()}
			allowMultiUserServerConfigurationEdit
			onOAuthConfigured={() => {
				if (!mcpServer) return;
				AdminService.getMCPCatalogServer(serverScopeID, mcpServer.id).then((server) => {
					mcpServer = server;
				});
			}}
			hideActions
		/>
		<button
			class="btn btn-primary"
			onclick={() => connectUrlDialog?.open(catalogEntry, mcpServer?.connectURL, mcpServer)}
		>
			<Link2Icon class="size-4" /> Connect URL
		</button>
	{/snippet}

	<div class="flex h-full flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		<McpDeprecatedNotice {deprecated} variant="notification" />

		<McpServerEntryForm
			entry={mcpServer}
			type="multi"
			id={serverScopeID}
			entity={serverScopeEntity}
			readonly={profile.current.isAdminReadonly?.()}
			allowMultiUserServerConfigurationEdit
			excludeViews={['overview']}
		/>
	</div>
</Layout>

<McpConnectUrlDialog bind:this={connectUrlDialog} />

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
