<script lang="ts">
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import { getMCPDisplayName, supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { Info } from '@lucide/svelte';

	interface Props {
		catalogEntry?: MCPCatalogEntry;
		server: MCPCatalogServer;
	}

	let { catalogEntry, server }: Props = $props();
	let title = $derived(getMCPDisplayName(server, catalogEntry?.manifest.name));
	let supportsDetails = $derived(supportsMCPBackendDetails(server));
</script>

{#if server}
	<div class="flex flex-col gap-6">
		{#if catalogEntry?.manifest.runtime === 'composite'}
			<McpServerCompositeInfo
				mcpServerId={server.id}
				name={title}
				entity="catalog"
				entityId={DEFAULT_MCP_CATALOG_ID}
				{catalogEntry}
				connectedUsers={[]}
			/>
		{:else if supportsDetails}
			<McpServerK8sInfo
				mcpServerId={server.id}
				name={title}
				connectedUsers={[]}
				readonly
				{catalogEntry}
				mcpServer={server}
				compositeParentName={server?.compositeName}
				hideTitle
				entity={server.powerUserWorkspaceID ? 'workspace' : 'catalog'}
				id={server.powerUserWorkspaceID || server.mcpCatalogID || DEFAULT_MCP_CATALOG_ID}
			/>
			{#if server?.manifest.runtime === 'remote'}
				<OAuthMetadataDebug metadata={server.oauthMetadata} />
			{/if}
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
