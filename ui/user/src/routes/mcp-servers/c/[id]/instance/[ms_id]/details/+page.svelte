<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerDetails from '$lib/components/mcp/McpServerDetails.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { UserService, type MCPCatalogServer, type OrgUser } from '$lib/services/index.js';
	import { getMCPDisplayName } from '$lib/services/user/mcp.js';
	import { profile } from '$lib/stores/index.js';
	import { Info } from '@lucide/svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let connectedUsers = $state<OrgUser[]>([]);

	// Make these reactive to data changes when navigating
	let catalogEntry = $derived(data.catalogEntry);
	let mcpServerId = $derived(data.mcpServerId);
	let mcpServer = $state<MCPCatalogServer>();
	let catalogEntryName = $derived(catalogEntry?.manifest?.name ?? 'Unknown');

	async function fetchUserInfo() {
		mcpServer = await UserService.getSingleOrRemoteMcpServer(mcpServerId);
		const isSameUser =
			connectedUsers.length === 1 ? connectedUsers[0].id === mcpServer.userID : false;

		if (mcpServer.userID && !isSameUser) {
			const user = await UserService.getUser(mcpServer.userID);
			connectedUsers = [user];
		}
	}

	$effect(() => {
		if (mcpServerId) {
			fetchUserInfo();
		}
	});

	let title = $derived(`${getMCPDisplayName(mcpServer, catalogEntryName)} | ${mcpServerId}`);
</script>

<Layout {title} showBackButton>
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServerId}
			{#if mcpServer && mcpServerId}
				<McpServerDetails
					serverId={mcpServerId}
					{connectedUsers}
					readonly={profile.current.isAdminReadonly?.()}
					{catalogEntry}
					server={mcpServer}
					k8sOverrides={{
						title
					}}
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
