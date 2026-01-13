<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		ChatService,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { Info } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let connectedUsers = $state<OrgUser[]>([]);

	// Make these reactive to data changes when navigating
	let catalogEntry = $derived(data.catalogEntry);
	let mcpServerId = $derived(data.mcpServerId);
	let compositeParentName = $state<string | undefined>();
	let mcpServer = $state<MCPCatalogServer>();
	let catalogEntryName = $derived(catalogEntry?.manifest?.name ?? 'Unknown');

	async function fetchUserInfo() {
		mcpServer = await ChatService.getSingleOrRemoteMcpServer(mcpServerId);
		const isSameUser =
			connectedUsers.length === 1 ? connectedUsers[0].id === mcpServer.userID : false;
		compositeParentName = mcpServer.compositeName;

		if (mcpServer.userID && !isSameUser) {
			const user = await AdminService.getUser(mcpServer.userID);
			connectedUsers = [user];
		}
	}

	$effect(() => {
		if (mcpServerId) {
			fetchUserInfo();
		}
	});

	let title = $derived(`${catalogEntryName} | ${mcpServerId}`);
</script>

<Layout {title} showBackButton>
	{#snippet rightNavActions()}
		<McpServerActions server={mcpServer} entry={catalogEntry} />
	{/snippet}
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServerId}
			{#if catalogEntry?.manifest.runtime === 'composite'}
				<McpServerCompositeInfo
					{mcpServerId}
					name={catalogEntryName}
					{connectedUsers}
					entity="catalog"
					entityId={DEFAULT_MCP_CATALOG_ID}
					{catalogEntry}
				/>
			{:else}
				<McpServerK8sInfo
					{mcpServerId}
					name={catalogEntryName}
					{connectedUsers}
					readonly={profile.current.isAdminReadonly?.()}
					{catalogEntry}
					{mcpServer}
					{compositeParentName}
				/>
			{/if}
		{:else}
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<p>Server information cannot be provided at this time.</p>
				</div>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
