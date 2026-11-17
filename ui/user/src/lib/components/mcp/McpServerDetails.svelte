<script lang="ts">
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import SubjectName from '$lib/components/admin/SubjectName.svelte';
	import McpTunnelDisconnectedStatus from '$lib/components/mcp/McpTunnelDisconnectedStatus.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { hasAgentOwner, instanceOwner } from '$lib/instanceOwners';
	import { createAgentLookup } from '$lib/runes/agentLookup.svelte';
	import type {
		MCPCatalogEntry,
		MCPCatalogServer,
		MCPServerInstance,
		OrgUser
	} from '$lib/services';
	import { getMCPDisplayName, supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { isMcpTunnelDisconnected } from '$lib/services/user/mcpTunnel';
	import { mcpTunnelConnections, profile } from '$lib/stores';
	import Table from '../table/Table.svelte';
	import { Info } from '@lucide/svelte';

	interface Props {
		catalogEntry?: MCPCatalogEntry;
		entity?: 'workspace' | 'catalog' | 'webhook-validation';
		entityId?: string;
		server?: MCPCatalogServer;
		serverId?: string;
		// The instances of the server. Each is owned by a user, or by an agent ("agent:<id>"), which is named
		// from the agents and never looked up as a user.
		instances?: (Pick<MCPServerInstance, 'userID'> & Partial<MCPServerInstance>)[];
		// The users who own instances, by ID.
		users?: Map<string, OrgUser>;
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
		instances = [],
		users = new Map(),
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
	const agentLookup = createAgentLookup(() =>
		hasAgentOwner(instances.map((instance) => instance.userID))
	);
	let connections = $derived(
		instances.map((instance) => ({
			...instanceOwner(instance.userID, users, agentLookup.current),
			mcpInstanceConfigured: instance.configured
		}))
	);
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
		{#if hasAdminAccess && entity !== 'webhook-validation' && connections.length > 0}
			<div>
				<h2 class="mb-2 text-lg font-semibold">
					{server?.serverUserType === 'multiUser'
						? m.mcps_servers_details_connected_users()
						: m.mcps_servers_details_associated_user()}
				</h2>
				<Table
					data={connections}
					fields={['name', 'updateStatus']}
					headers={[
						{ title: m.core_name(), property: 'name' },
						{ title: m.mcps_servers_details_config_status(), property: 'updateStatus' }
					]}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							<SubjectName name={d.name} disabled={d.disabled} agent={d.agent} />
						{:else if property === 'updateStatus'}
							{d.mcpInstanceConfigured === false
								? m.core_mcp_value_not_configured()
								: m.core_mcp_value_up_to_date()}
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
			<p>{m.mcps_servers_details_unavailable()}</p>
		</div>
	</div>
{/if}
