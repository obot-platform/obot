<script lang="ts">
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, ChatService, type MCPServerInstance, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { LoaderCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer, workspaceId } = data;
	let loading = $state(false);
	let users = $state<OrgUser[]>([]);
	let instances = $state<MCPServerInstance[]>([]);
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(async () => {
		if (!mcpServer) return;
		loading = true;
		instances = await ChatService.listWorkspaceMcpCatalogServerInstances(workspaceId, mcpServer.id);
		users = await AdminService.listUsersIncludeDeleted();
		loading = false;
	});

	let title = $derived(mcpServer?.manifest?.name ?? 'MCP Server Details');
</script>

<Layout {title} showBackButton>
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		{#if loading}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div class="flex flex-col gap-6">
				{#if mcpServer}
					<McpServerK8sInfo
						id={workspaceId}
						entity="workspace"
						mcpServerId={mcpServer.id}
						name={mcpServer.manifest.name || ''}
						connectedUsers={(instances ?? []).map((instance) => {
							const user = usersMap.get(instance.userID)!;
							return {
								...user,
								mcpInstanceId: instance.id
							};
						})}
						title="Details"
						classes={{
							title: 'text-lg font-semibold'
						}}
						readonly={profile.current.isAdminReadonly?.()}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
