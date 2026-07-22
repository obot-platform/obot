<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { Group, type MCPCatalogEntry, type MCPCatalogServer, type OrgUser } from '$lib/services';
	import { getMCPDisplayName, supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { isOwnSingleUserServer } from '$lib/utils';
	import Table from '../table/Table.svelte';
	import { Info } from '@lucide/svelte';

	interface Props {
		catalogEntry?: MCPCatalogEntry;
		entity?: 'workspace' | 'catalog' | 'agent' | 'webhook-validation';
		entityId?: string;
		server?: MCPCatalogServer;
		serverId?: string;
		connectedUsers?: (OrgUser & { mcpInstanceId?: string; mcpInstanceConfigured?: boolean })[];
		compositeParentName?: string;
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
		compositeParentName,
		k8sOverrides,
		readonly
	}: Props = $props();
	let title = $derived(
		k8sOverrides?.title ?? getMCPDisplayName(server, catalogEntry?.manifest.name)
	);
	let supportsDetails = $derived(supportsMCPBackendDetails(server));
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));
	let entity = $derived(
		overrideEntity ?? (server && server?.powerUserWorkspaceID ? 'workspace' : 'catalog')
	);
	let entityId = $derived(
		overrideEntityId ??
			server?.powerUserWorkspaceID ??
			server?.mcpCatalogID ??
			catalogEntry?.id ??
			DEFAULT_MCP_CATALOG_ID
	);
	let mcpServerId = $derived(serverId ?? server?.id);

	function getAuditLogUrl(d: OrgUser) {
		const id = serverId ?? server?.id;

		if (!id) return null;

		if (compositeParentName || entity === 'agent') return null;

		if (isAdminUrl) {
			const adminPrefix = page.url.pathname.startsWith('/admin/mcp-deployments')
				? '/admin/mcp-deployments'
				: '/admin/mcp-catalog';

			if (!hasAdminAccess) return null;
			return entity === 'workspace'
				? catalogEntry?.id
					? `${adminPrefix}/w/${entityId}/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `${adminPrefix}/w/${entityId}/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`
				: catalogEntry?.id
					? `${adminPrefix}/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `${adminPrefix}/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
		}

		// Basic users can access audit logs for their own single-user servers
		let isOwnServer = server && isOwnSingleUserServer(server, profile.current?.id);
		if (!isOwnServer && !profile.current?.groups.includes(Group.POWERUSER)) return null;
		return catalogEntry?.id
			? `/mcp-catalog/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
			: `/mcp-catalog/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
	}
</script>

{#if server || mcpServerId}
	<div class="flex flex-col gap-6">
		{#if catalogEntry?.manifest.runtime === 'composite'}
			<McpServerCompositeInfo
				{mcpServerId}
				name={title}
				entity="catalog"
				entityId={DEFAULT_MCP_CATALOG_ID}
				{catalogEntry}
				connectedUsers={[]}
			/>
		{:else if supportsDetails && mcpServerId}
			<McpServerK8sInfo
				{mcpServerId}
				name={title}
				{readonly}
				{catalogEntry}
				mcpServer={server}
				compositeParentName={server?.compositeName}
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

					{#snippet actions(d)}
						{@const auditLogsUrl = getAuditLogUrl(d)}
						{#if auditLogsUrl}
							<a href={resolve(auditLogsUrl as `/${string}`)} class="btn btn-link">
								View Audit Logs
							</a>
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
