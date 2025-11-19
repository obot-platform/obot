<script lang="ts">
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, ChatService, type OrgUser } from '$lib/services/index.js';
	import { Info } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	let { catalogEntry, mcpServerId, workspaceId } = data;
	const duration = PAGE_TRANSITION_DURATION;
	let connectedUsers = $state<OrgUser[]>([]);

	let catalogEntryName = catalogEntry?.manifest?.name ?? 'Unknown';

	async function fetchUserInfo() {
		if (!workspaceId || !catalogEntry?.id) return;
		const mcpServer = await ChatService.getWorkspaceCatalogEntryServer(
			workspaceId,
			catalogEntry.id,
			mcpServerId
		);
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
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServerId && catalogEntry?.manifest.runtime !== 'remote'}
			<McpServerK8sInfo
				id={workspaceId}
				entity="workspace"
				{catalogEntry}
				{mcpServerId}
				name={catalogEntryName}
				{connectedUsers}
				readonly={profile.current.isAdminReadonly?.()}
			/>
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
	<title>Obot | {title}</title>
</svelte:head>
