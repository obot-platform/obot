<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerDetails from '$lib/components/mcp/McpServerDetails.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, UserService, type MCPServerInstance, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer } = $derived(data);
	let workspaceId = $derived(mcpServer?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(workspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(workspaceId || mcpServer?.mcpCatalogID || DEFAULT_MCP_CATALOG_ID);
	let loading = $state(false);
	let users = $state<OrgUser[]>([]);
	let instances = $state<MCPServerInstance[]>([]);
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(async () => {
		if (!mcpServer) return;
		loading = true;
		instances = workspaceId
			? await UserService.listWorkspaceMcpCatalogServerInstances(workspaceId, mcpServer.id)
			: await AdminService.listMcpCatalogServerInstances(serverScopeID, mcpServer.id);
		users = await UserService.listUsersIncludeDeleted();
		loading = false;
	});
	let title = $derived(mcpServer?.manifest.name);
</script>

<Layout {title} showBackButton>
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		{#if loading}
			<div class="flex w-full justify-center">
				<Loading class="size-6" />
			</div>
		{:else}
			<div class="flex flex-col gap-6">
				{#if mcpServer}
					<McpServerDetails
						entityId={serverScopeID}
						entity={serverScopeEntity}
						serverId={mcpServer.id}
						connectedUsers={(instances ?? []).map((instance) => {
							const user = usersMap.get(instance.userID)!;
							return {
								...user,
								mcpInstanceId: instance.id,
								mcpInstanceConfigured: instance.configured
							};
						})}
						readonly={profile.current.isAdminReadonly?.()}
						server={mcpServer}
						compositeParentName={mcpServer.compositeName}
						k8sOverrides={{
							title: mcpServer.manifest.name
						}}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
