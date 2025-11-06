<script lang="ts">
	import BackLink from '$lib/components/BackLink.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, ChatService, type OrgUser } from '$lib/services/index.js';
	import { Info } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { profile } from '$lib/stores/index.js';
	import { page } from '$app/state';
	import McpServerRemoteInfo from '$lib/components/admin/McpServerRemoteInfo.svelte';
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';

	let { data } = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let connectedUsers = $state<OrgUser[]>([]);

	// Make these reactive to data changes when navigating
	let catalogEntry = $derived(data.catalogEntry);
	let mcpServerId = $derived(data.mcpServerId);
	let compositeParentName = $state<string | undefined>();
	let catalogEntryName = $derived(catalogEntry?.manifest?.name ?? 'Unknown');

	async function fetchUserInfo() {
		const mcpServer = await ChatService.getSingleOrRemoteMcpServer(mcpServerId);
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
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServerId}
			{@const currentLabel = mcpServerId ?? 'Server'}
			{@const from = page.url.searchParams.get('from') ?? `/mcp-servers/${catalogEntry?.id}`}
			{#key from}
				<BackLink fromURL={from} {currentLabel} serverId={mcpServerId} />
			{/key}
		{/if}

		{#if mcpServerId}
			{#if catalogEntry?.manifest.runtime === 'remote'}
				<McpServerRemoteInfo
					{mcpServerId}
					name={catalogEntryName}
					{connectedUsers}
					entity="catalog"
					entityId={DEFAULT_MCP_CATALOG_ID}
					{catalogEntry}
					{compositeParentName}
				/>
			{:else if catalogEntry?.manifest.runtime === 'composite'}
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
					{compositeParentName}
				/>
			{/if}
		{:else}
			<h1 class="text-2xl font-semibold">
				{catalogEntryName} | {mcpServerId}
			</h1>

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
	<title>Obot | {catalogEntryName} | {mcpServerId}</title>
</svelte:head>
