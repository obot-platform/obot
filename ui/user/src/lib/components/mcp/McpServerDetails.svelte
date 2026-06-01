<script lang="ts">
	import McpServerCompositeInfo from '$lib/components/admin/McpServerCompositeInfo.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import OAuthMetadataDebug from '$lib/components/mcp/OAuthMetadataDebug.svelte';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import { getMCPDisplayName } from '$lib/services/user/mcp';
	import { Info } from 'lucide-svelte';

	interface Props {
		catalogEntry?: MCPCatalogEntry;
		server: MCPCatalogServer;
	}

	let { catalogEntry, server }: Props = $props();
	let title = $derived(getMCPDisplayName(server, catalogEntry?.manifest.name));
</script>

{#if server}
	<div class="flex flex-col gap-6">
		{#if catalogEntry?.manifest.runtime === 'composite'}
			<McpServerCompositeInfo
				mcpServerId={server.id}
				name={title}
				entity="workspace"
				entityId={server.powerUserWorkspaceID}
				{catalogEntry}
				connectedUsers={[]}
			/>
		{:else}
			<McpServerK8sInfo
				mcpServerId={server.id}
				name={title}
				connectedUsers={[]}
				readonly
				{catalogEntry}
				mcpServer={server}
				compositeParentName={server?.compositeName}
				hideTitle
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
