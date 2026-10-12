<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import DeploymentsView from '../mcp/DeploymentsView.svelte';
	import McpServerDetails from '../mcp/McpServerDetails.svelte';
	import { Router } from '@lucide/svelte';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		catalogEntry?: MCPCatalogEntry;
		users?: OrgUser[];
		type?: LaunchServerType;
		configuredInstances?: MCPServerInstance[];
		configuredServers?: MCPCatalogServer[];
		loading?: boolean;
		onReload?: () => void | Promise<void>;
	}

	let {
		id,
		entity = 'catalog',
		entry,
		users = [],
		type,
		configuredInstances = [],
		configuredServers = [],
		loading = false,
		onReload
	}: Props = $props();

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));
	let readonly = $derived(profile.current.isAdminReadonly?.());
</script>

{#if loading}
	<div class="flex w-full justify-center">
		<Loading class="size-6" />
	</div>
{:else if entry && !('isCatalogEntry' in entry) && id}
	{#if entry && (type === 'multi' || configuredInstances.length > 0)}
		<McpServerDetails
			{entity}
			entityId={id}
			server={entry}
			connectedUsers={configuredInstances.map((instance) => {
				const user = usersMap.get(instance.userID)!;
				return {
					...user,
					mcpInstanceId: instance.id,
					mcpInstanceConfigured: instance.configured
				};
			})}
			k8sOverrides={{
				title: m.core_details(),
				classes: {
					title: 'text-lg font-semibold'
				}
			}}
			{readonly}
		/>
	{:else}
		{@render emptyInstancesContent()}
	{/if}
{:else}
	{#if configuredServers.length > 0}
		<DeploymentsView
			servers={configuredServers}
			{readonly}
			{id}
			{entity}
			{usersMap}
			{onReload}
			skipLoadMcpsOnMount
			{entry}
		/>
	{:else}
		{@render emptyInstancesContent()}
	{/if}
{/if}

{#snippet emptyInstancesContent()}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Router class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">{m.mcps_servers_no_server_details()}</h4>
		<p class="text-muted-content text-sm font-light">
			{m.mcps_servers_no_details_for_entry()}
		</p>
	</div>
{/snippet}
