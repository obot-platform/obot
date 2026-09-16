<script lang="ts">
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import McpTunnelDisconnectedStatus from '$lib/components/mcp/McpTunnelDisconnectedStatus.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import type { MCPCatalogEntry, MCPCatalogServer, OrgUser } from '$lib/services';
	import { getMCPDisplayName, supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { isMcpTunnelDisconnected } from '$lib/services/user/mcpTunnel';
	import { mcpTunnelConnections, profile } from '$lib/stores';
	import Table from '../table/Table.svelte';
	import { Info } from '@lucide/svelte';

	interface Props {
		catalogEntry?: MCPCatalogEntry;
		entity?: 'workspace' | 'catalog' | 'agent' | 'webhook-validation';
		entityId?: string;
		server?: MCPCatalogServer;
		serverId?: string;
		connectedUsers?: (OrgUser & { mcpInstanceId?: string; mcpInstanceConfigured?: boolean })[];
		k8sOverrides?: {
			title?: string;
			classes?: {
				title?: string;
			};
		};
		readonly?: boolean;
	}

	let {
		catalogEntry,
		entity: overrideEntity,
		entityId: overrideEntityId,
		server,
		serverId,
		connectedUsers,
		k8sOverrides,
		readonly
	}: Props = $props();
	let title = $derived(
		k8sOverrides?.title ?? getMCPDisplayName(server, catalogEntry?.manifest.name)
	);
	let supportsDetails = $derived(supportsMCPBackendDetails(server));
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let entity = $derived(
		overrideEntity ??
			(server?.powerUserWorkspaceID || catalogEntry?.powerUserWorkspaceID ? 'workspace' : 'catalog')
	);
	let entityId = $derived(
		overrideEntityId ??
			server?.powerUserWorkspaceID ??
			catalogEntry?.powerUserWorkspaceID ??
			server?.mcpCatalogID ??
			catalogEntry?.id ??
			DEFAULT_MCP_CATALOG_ID
	);
	let mcpServerId = $derived(serverId ?? server?.id);
	let tunnelDisconnected = $derived(
		isMcpTunnelDisconnected(server ?? catalogEntry, mcpTunnelConnections.current.connections)
	);
</script>

{#if server || mcpServerId}
	<div class="flex flex-col gap-6">
		{#if tunnelDisconnected}
			<McpTunnelDisconnectedStatus detailed />
		{/if}
		{#if supportsDetails && mcpServerId}
			<McpServerK8sInfo
				{mcpServerId}
				name={title}
				{readonly}
				{catalogEntry}
				mcpServer={server}
				hideTitle
				{entity}
				id={entityId}
				{...k8sOverrides}
			/>
		{/if}
		{#if hasAdminAccess && entity !== 'webhook-validation' && connectedUsers && connectedUsers.length > 0}
			<div>
				<h2 class="mb-2 text-lg font-semibold">
					{server?.serverUserType === 'multiUser' ? 'Connected Users' : 'Associated User'}
				</h2>
				<Table
					data={connectedUsers ?? []}
					fields={['name', 'updateStatus']}
					headers={[{ title: 'Config Status', property: 'updateStatus' }]}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							{d.email || d.username || 'Unknown'}
						{:else if property === 'updateStatus'}
							{d.mcpInstanceConfigured === false ? 'Not Configured' : 'Up to date'}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
				</Table>
			</div>
		{/if}
		{#if server?.manifest.runtime === 'remote'}
			<OAuthMetadataDebug metadata={server.oauthMetadata} />
		{/if}
	</div>
{:else}
	<div class="notification-info p-3 text-sm font-light">
		<div class="flex items-center gap-3">
			<Info class="size-6" />
			<p>Server information cannot be provided at this time.</p>
		</div>
	</div>
{/if}
