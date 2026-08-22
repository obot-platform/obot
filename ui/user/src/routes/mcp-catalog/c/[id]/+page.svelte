<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import { isDeprecatedMCPServer, isMultiUserCatalogEntry } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import McpConnectUrlDialog from '../../McpConnectUrlDialog.svelte';
	import { Link2Icon } from '@lucide/svelte';
	import { untrack, type Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let catalogEntry = $state(untrack(() => data.catalogEntry));

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isSourcedEntry = $derived(
		catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL
	);
	let deprecated = $derived(isDeprecatedMCPServer(catalogEntry));

	let workspaceId = $derived(catalogEntry?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(workspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(workspaceId || DEFAULT_MCP_CATALOG_ID);

	let connectUrlDialog = $state<ReturnType<typeof McpConnectUrlDialog>>();
	let mcpServerActions = $state<ReturnType<typeof McpServerActions>>();
	let showUrlOnConnect = $state(false);

	async function acceptOwnership() {
		if (!catalogEntry) return;
		catalogEntry = await AdminService.acceptMCPCatalogEntryOwnership(
			DEFAULT_MCP_CATALOG_ID,
			catalogEntry.id
		);
	}

	let title = $derived(catalogEntry?.manifest?.name ?? 'MCP Server');
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
		<McpServerActions
			bind:this={mcpServerActions}
			entry={catalogEntry}
			catalogID={workspaceId ? undefined : serverScopeID}
			workspaceID={workspaceId}
			{promptInitialLaunch}
			{promptOAuthConfig}
			onOAuthConfigured={() => {
				if (!catalogEntry) return;
				AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, catalogEntry.id).then((entry) => {
					catalogEntry = entry;
				});
			}}
			onConnect={({ entry, server }) => {
				if (isMultiUserCatalogEntry(entry) && server) {
					success.add(`${server.alias || server.manifest.name} has been created.`);
				}
				if (showUrlOnConnect) {
					showUrlOnConnect = false;
					connectUrlDialog?.open(entry, server?.connectURL, server);
				}
			}}
			hideActions
		/>
		<button class="btn btn-primary" onclick={() => connectUrlDialog?.open(catalogEntry)}>
			<Link2Icon class="size-4" /> Connect URL
		</button>
	{/snippet}
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpDeprecatedNotice {deprecated} variant="notification" />
		{#if profile.current.hasAdminAccess?.()}
			<McpDetachedNotice
				detached={catalogEntry?.detached}
				sourceURL={catalogEntry?.sourceURL}
				variant="notification"
				onAcceptOwnership={isAdminReadonly ? undefined : acceptOwnership}
			/>
		{/if}

		<McpServerEntryForm
			entry={catalogEntry}
			type={catalogEntry?.manifest.runtime === 'composite'
				? 'composite'
				: catalogEntry?.manifest.runtime === 'remote'
					? 'remote'
					: 'hosted'}
			readonly={isAdminReadonly || isSourcedEntry}
			id={serverScopeID}
			entity={serverScopeEntity}
			excludeViews={['overview']}
		/>
	</div>
</Layout>

<McpConnectUrlDialog
	bind:this={connectUrlDialog}
	onLaunchCatalogEntry={() => {
		showUrlOnConnect = true;
		mcpServerActions?.connect();
	}}
/>

<svelte:head>
	<title>Obot | {catalogEntry?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
