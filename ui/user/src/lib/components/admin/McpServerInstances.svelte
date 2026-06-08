<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';
	import { hasMissingSecretBindingConfig, isMultiUserServer } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import DeploymentsView from '../mcp/DeploymentsView.svelte';
	import McpServerK8sInfo from './McpServerK8sInfo.svelte';
	import { CircleFadingArrowUp, Router } from 'lucide-svelte';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		catalogEntry?: MCPCatalogEntry;
		users?: OrgUser[];
		type?: LaunchServerType;
		configuredServers?: MCPCatalogServer[];
	}

	let {
		id,
		entity = 'catalog',
		entry,
		catalogEntry,
		users = [],
		type,
		configuredServers
	}: Props = $props();

	let instances = $state<MCPServerInstance[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let loading = $state(true);

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));
	let detailsCatalogEntry = $derived(
		catalogEntry ?? (entry && 'isCatalogEntry' in entry ? entry : undefined)
	);
	let detailsMcpServer = $derived(entry && !('isCatalogEntry' in entry) ? entry : undefined);
	let readonly = $derived(profile.current.isAdminReadonly?.());

	$effect(() => {
		if (!loading) return;
		if (entry && !('isCatalogEntry' in entry) && id) {
			if (entry.catalogEntryID && !isMultiUserServer(entry)) {
				instances = [
					{
						id: entry.id,
						configured: entry.configured,
						missingRequiredHeaders: entry.missingRequiredHeader,
						userID: entry.userID,
						created: entry.created
					}
				];
				loading = false;
			} else {
				if (entity === 'workspace') {
					UserService.listWorkspaceMcpCatalogServerInstances(id, entry.id)
						.then((response) => {
							instances = response;
						})
						.finally(() => {
							loading = false;
						});
				} else {
					AdminService.listMcpCatalogServerInstances(id, entry.id)
						.then((response) => {
							instances = response;
						})
						.finally(() => {
							loading = false;
						});
				}
			}
		} else if (entry && 'isCatalogEntry' in entry) {
			if (configuredServers && configuredServers.length > 0) {
				const filtered = configuredServers.filter((s) => s.catalogEntryID === entry.id);
				servers = filtered;
				loading = false;
			} else if (id) {
				if (entity === 'workspace') {
					UserService.listWorkspaceMCPServersForEntry(id, entry.id)
						.then((response) => {
							servers = response;
						})
						.finally(() => {
							loading = false;
						});
				} else {
					AdminService.listMCPServersForEntry(id, entry.id)
						.then((response) => {
							servers = response;
						})
						.finally(() => {
							loading = false;
						});
				}
			}
		}
	});

	function isMissingKubernetesSecret(server: MCPCatalogServer) {
		return hasMissingSecretBindingConfig(
			server.manifest,
			server.missingRequiredEnvVars,
			server.missingRequiredHeader
		);
	}

	async function reload() {
		if (!id || !entry) return;
		if (entity === 'workspace') {
			UserService.listWorkspaceMCPServersForEntry(id, entry.id)
				.then((response) => {
					servers = response;
				})
				.finally(() => {
					loading = false;
				});
		} else {
			AdminService.listMCPServersForEntry(id, entry.id)
				.then((response) => {
					servers = response;
				})
				.finally(() => {
					loading = false;
				});
		}
	}
</script>

{#if loading}
	<div class="flex w-full justify-center">
		<Loading class="size-6" />
	</div>
{:else if entry && !('isCatalogEntry' in entry) && id}
	{#if entry && (type === 'multi' || instances.length > 0)}
		<div class="flex flex-col gap-6">
			<McpServerK8sInfo
				{id}
				{entity}
				mcpServerId={entry.id}
				name={'manifest' in entry ? entry.manifest.name || '' : ''}
				catalogEntry={detailsCatalogEntry}
				mcpServer={detailsMcpServer}
				connectedUsers={instances.map((instance) => {
					const user = usersMap.get(instance.userID)!;
					return {
						...user,
						mcpInstanceId: instance.id,
						mcpInstanceConfigured: instance.configured
					};
				})}
				title="Details"
				classes={{
					title: 'text-lg font-semibold'
				}}
				{readonly}
			/>
		</div>
	{:else}
		{@render emptyInstancesContent()}
	{/if}
{:else}
	{@const numServerUpdatesNeeded = servers.filter(
		(s) => s.needsUpdate && !isMissingKubernetesSecret(s)
	).length}
	{#if servers.length > 0}
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
		<DeploymentsView {servers} {readonly} {id} {entity} {usersMap} onReload={reload} />
	{:else}
		{@render emptyInstancesContent()}
	{/if}
{/if}

{#snippet emptyInstancesContent()}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Router class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No server details</h4>
		<p class="text-muted-content text-sm font-light">No details available yet for this server.</p>
	</div>
{/snippet}
