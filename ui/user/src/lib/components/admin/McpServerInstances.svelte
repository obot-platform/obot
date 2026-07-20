<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import {
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';
	import { hasMissingSecretBindingConfig } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import DeploymentsView from '../mcp/DeploymentsView.svelte';
	import McpServerDetails from '../mcp/McpServerDetails.svelte';
	import { CircleFadingArrowUp, Router } from '@lucide/svelte';

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

	function isMissingKubernetesSecret(server: MCPCatalogServer) {
		return hasMissingSecretBindingConfig(
			server.manifest,
			server.missingRequiredEnvVars,
			server.missingRequiredHeader
		);
	}
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
				title: 'Details',
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
	{@const numServerUpdatesNeeded = configuredServers.filter(
		(s) => s.needsUpdate && !isMissingKubernetesSecret(s)
	).length}
	{#if configuredServers.length > 0}
		{#if numServerUpdatesNeeded}
			<div class="group bg-base-100 mb-2 w-fit rounded-md">
				<div
					class="border-primary bg-primary/10 group-hover:bg-primary/20 dark:bg-primary/30 dark:group-hover:bg-primary/40 flex items-center gap-1 rounded-md border px-4 py-2 transition-colors duration-300"
				>
					<CircleFadingArrowUp class="text-primary size-4" />
					<p class="text-primary text-sm font-light">
						{#if numServerUpdatesNeeded === 1}
							1 deployment has an update available.
						{:else}
							{numServerUpdatesNeeded} deployments have updates available.
						{/if}
					</p>
				</div>
			</div>
		{/if}
		<DeploymentsView
			servers={configuredServers}
			{readonly}
			{id}
			{entity}
			{usersMap}
			{onReload}
			skipLoadOnMount
		/>
	{:else}
		{@render emptyInstancesContent()}
	{/if}
{/if}

{#snippet emptyInstancesContent()}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Router class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No server details</h4>
		<p class="text-muted-content text-sm font-light">No details available yet for this entry.</p>
	</div>
{/snippet}
