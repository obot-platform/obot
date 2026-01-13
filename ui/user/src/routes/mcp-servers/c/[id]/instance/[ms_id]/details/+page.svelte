<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		ChatService,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { Info } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { catalogEntry, mcpServerId, workspaceId } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;
	let connectedUsers = $state<OrgUser[]>([]);
	let mcpServer = $state<MCPCatalogServer>();

	let catalogEntryName = $derived(catalogEntry?.manifest?.name ?? 'Unknown');

	async function fetchUserInfo() {
		if (!mcpServerId) return;
		mcpServer =
			catalogEntry && catalogEntry.powerUserWorkspaceID && workspaceId
				? await ChatService.getWorkspaceCatalogEntryServer(
						workspaceId,
						catalogEntry.id,
						mcpServerId
					)
				: await ChatService.getSingleOrRemoteMcpServer(mcpServerId);
		if (mcpServer?.userID) {
			const user = await AdminService.getUser(mcpServer.userID);
			connectedUsers = [user];
		}
	}

	onMount(() => {
		fetchUserInfo();
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
					entity="workspace"
					entityId={workspaceId}
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
					compositeParentName={mcpServer?.compositeName}
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
