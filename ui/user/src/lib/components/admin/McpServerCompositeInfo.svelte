<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ADMIN_SESSION_STORAGE, DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import {
		AdminService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import { isDeprecatedMCPServer, supportsMCPBackendDetails } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { openUrl } from '$lib/utils';
	import McpDeprecatedNotice from '../mcp/McpDeprecatedNotice.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import { CircleAlert, ChevronRight, Server } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		entity?: 'workspace' | 'catalog';
		entityId?: string;
		catalogEntry?: MCPCatalogEntry;
		mcpServerId?: string;
		mcpServerInstanceId?: string;
		classes?: {
			title?: string;
		};
		name: string;
		connectedUsers: OrgUser[];
		hideTitle?: boolean;
	}

	let { name, connectedUsers, classes, entityId, catalogEntry, mcpServerId, hideTitle }: Props =
		$props();
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));
	let servers = $state<MCPCatalogServer[]>([]);
	let loadingServers = $state(true);
	let failedToLoadServers = $state(false);
	let serversMap = $derived(new Map(servers.map((s) => [s.catalogEntryID || s.id, s])));

	onMount(async () => {
		if (!mcpServerId || !catalogEntry?.id || !entityId) {
			loadingServers = false;
			return;
		}

		try {
			const [deployedCatalogEntryServers, deployedWorkspaceCatalogEntryServers] = await Promise.all(
				[
					AdminService.listAllCatalogDeployedSingleRemoteServers(DEFAULT_MCP_CATALOG_ID),
					AdminService.listAllWorkspaceDeployedSingleRemoteServers()
				]
			);

			servers = [
				...deployedCatalogEntryServers.filter((s) => s.compositeName === mcpServerId),
				...deployedWorkspaceCatalogEntryServers.filter((s) => s.compositeName === mcpServerId)
			];
		} catch {
			failedToLoadServers = true;
		} finally {
			loadingServers = false;
		}
	});
</script>

{#if !hideTitle}
	<div class="flex items-center gap-3">
		<h1 class={twMerge('text-2xl font-semibold', classes?.title)}>
			{name}
		</h1>
	</div>
{/if}

{#if catalogEntry?.manifest.compositeConfig?.componentServers}
	<div>
		<h2 class="mb-2 text-lg font-semibold">MCP Servers</h2>
		<div class="flex flex-col gap-2">
			{#each catalogEntry.manifest.compositeConfig.componentServers as componentServer (componentServer.catalogEntryID || componentServer.mcpServerID)}
				{@const catalogEntryServerId =
					componentServer.catalogEntryID && serversMap.get(componentServer.catalogEntryID)?.id}
				{@const multiUserServerId = componentServer.mcpServerID}
				{@const componentServerId = catalogEntryServerId || multiUserServerId}
				{@const componentExists = !!componentServerId}
				{@const deprecated = isDeprecatedMCPServer(componentServer)}

				{#if componentExists}
					<button
						onclick={(e) => {
							const isCtrlClick = e.metaKey || e.ctrlKey;
							const supportsDetails = supportsMCPBackendDetails(componentServer);
							const url = componentServer.catalogEntryID
								? `/admin/mcp-catalog/c/${componentServer.catalogEntryID}/instance/${catalogEntryServerId}${supportsDetails ? '/details' : ''}`
								: `/admin/mcp-catalog/s/${componentServer.mcpServerID}${supportsDetails ? '/details' : ''}`;

							sessionStorage.setItem(
								ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
								JSON.stringify({
									id: catalogEntry?.id,
									name,
									type: 'composite',
									entity: 'catalog',
									entityId: DEFAULT_MCP_CATALOG_ID,
									serverId: mcpServerId
								})
							);

							openUrl(url, isCtrlClick);
						}}
						class="dark:bg-base-200 dark:border-base-400 dark:hover:bg-base-200 bg-base-100 flex items-center justify-between gap-2 rounded-lg border border-transparent p-2 pl-4 shadow-sm hover:bg-gray-50"
					>
						<div class="flex items-center gap-2">
							<div class="icon">
								{#if componentServer.manifest?.icon}
									<img
										src={componentServer.manifest?.icon}
										alt={componentServer.manifest?.name}
										class="size-6"
									/>
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<p class="text-sm">{componentServer.manifest?.name}</p>
							<McpDeprecatedNotice {deprecated} child />
							{#if componentServerId}
								<span class="text-muted-content text-sm">({componentServerId})</span>
							{/if}
						</div>
						<IconButton>
							<ChevronRight class="size-6" />
						</IconButton>
					</button>
				{:else}
					<div
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex items-center justify-between gap-2 rounded-lg border border-transparent p-2 pl-4 opacity-60 shadow-sm"
					>
						<div class="flex items-center gap-2">
							<div class="icon">
								{#if componentServer.manifest?.icon}
									<img
										src={componentServer.manifest?.icon}
										alt={componentServer.manifest?.name}
										class="size-6"
									/>
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<p class="text-sm">{componentServer.manifest?.name}</p>
							<McpDeprecatedNotice {deprecated} child />
							{#if loadingServers}
								<span class="text-muted-content text-xs">Loading...</span>
							{:else if failedToLoadServers}
								<span
									class="text-muted-content flex items-center gap-1 text-xs"
									title="Unable to determine whether this component server still exists"
								>
									<CircleAlert class="size-4" />
									<span>Unavailable</span>
								</span>
							{:else}
								<span
									class="text-muted-content flex items-center gap-1 text-xs"
									title="This component server no longer exists"
								>
									<CircleAlert class="size-4" />
									<span>Deleted</span>
								</span>
							{/if}
						</div>
						<div class="size-10 shrink-0"></div>
					</div>
				{/if}
			{/each}
		</div>
	</div>
{/if}

<div>
	<h2 class="mb-2 text-lg font-semibold">Connected Users</h2>

	<!-- show connected URL, configuration settings -->
	<Table data={connectedUsers} fields={['name']}>
		{#snippet onRenderColumn(property: string, d: OrgUser)}
			{#if property === 'name'}
				{d.email || d.username || 'Unknown'}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}

		{#snippet actions(d)}
			{#if catalogEntry?.id && isAdminUrl && profile.current?.hasAdminAccess?.()}
				<a
					href={resolve(
						`/admin/mcp-catalog/c/${catalogEntry.id}?view=audit-logs&user_id=${encodeURIComponent(d.id)}`
					)}
					class="btn btn-link"
				>
					View Audit Logs
				</a>
			{/if}
		{/snippet}
	</Table>
</div>
