<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, UserService, type MCPServerInstance, type OrgUser } from '$lib/services';
	import { supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores/index.js';
	import { Info } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer } = $derived(data);
	let workspaceId = $derived(mcpServer?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(workspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(workspaceId || mcpServer?.mcpCatalogID || DEFAULT_MCP_CATALOG_ID);
	let supportsDetails = $derived(supportsMCPBackendDetails(mcpServer));
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
					{#if supportsDetails}
						<McpServerK8sInfo
							id={serverScopeID}
							entity={serverScopeEntity}
							mcpServerId={mcpServer.id}
							{mcpServer}
							name={mcpServer.manifest.name || ''}
							connectedUsers={(instances ?? []).map((instance) => {
								const user = usersMap.get(instance.userID)!;
								return {
									...user,
									mcpInstanceId: instance.id,
									mcpInstanceConfigured: instance.configured
								};
							})}
							title={mcpServer.manifest.name}
							readonly={profile.current.isAdminReadonly?.()}
						/>
					{:else}
						<div class="notification-info p-3 text-sm font-light">
							<div class="flex items-center gap-3">
								<Info class="size-6" />
								<p>Server details are not available for this MCP server runtime.</p>
							</div>
						</div>
					{/if}
					{#if mcpServer.manifest.runtime === 'remote'}
						<OAuthMetadataDebug metadata={mcpServer.oauthMetadata} />
					{/if}
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
