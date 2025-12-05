<script lang="ts">
	import { page } from '$app/state';
	import {
		AdminService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import Table from '../table/Table.svelte';
	import { onMount } from 'svelte';
	import { ChevronRight, Server } from 'lucide-svelte';
	import { ADMIN_SESSION_STORAGE, DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { openUrl } from '$lib/utils';

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
	}

	let { name, connectedUsers, classes, entityId, catalogEntry, mcpServerId }: Props = $props();
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));
	let servers = $state<MCPCatalogServer[]>([]);
	let serversMap = $derived(new Map(servers.map((s) => [s.catalogEntryID || s.id, s])));

	onMount(async () => {
		if (!mcpServerId || !catalogEntry?.id || !entityId) return;

		const deployedCatalogEntryServers =
			await AdminService.listAllCatalogDeployedSingleRemoteServers(DEFAULT_MCP_CATALOG_ID);
		const deployedWorkspaceCatalogEntryServers =
			await AdminService.listAllWorkspaceDeployedSingleRemoteServers();

		servers = [
			...deployedCatalogEntryServers.filter((s) => s.compositeName === mcpServerId),
			...deployedWorkspaceCatalogEntryServers.filter((s) => s.compositeName === mcpServerId)
		];
	});

	function getAuditLogUrl(d: OrgUser) {
		if (!catalogEntry?.id) return null;
		if (!isAdminUrl) return null;
		if (!profile.current?.hasAdminAccess?.()) return null;
		return `/admin/mcp-servers/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`;
	}
</script>

<div class="flex items-center gap-3">
	<h1 class={twMerge('text-2xl font-semibold', classes?.title)}>
		{name}
	</h1>
</div>

{#if catalogEntry?.manifest.compositeConfig?.componentServers}
	<div>
		<h2 class="mb-2 text-lg font-semibold">MCP Servers</h2>
		<div class="flex flex-col gap-2">
			{#each catalogEntry.manifest.compositeConfig.componentServers as componentServer (componentServer.catalogEntryID)}
				{@const catalogEntryServerId =
					componentServer.catalogEntryID && serversMap.get(componentServer.catalogEntryID)?.id}
				<button
					onclick={(e) => {
						const isCtrlClick = e.metaKey || e.ctrlKey;
						const url = componentServer.catalogEntryID
							? `/admin/mcp-servers/c/${componentServer.catalogEntryID}/instance/${serversMap.get(componentServer.catalogEntryID)?.id}`
							: `/admin/mcp-servers/s/${componentServer.mcpServerID}/details`;

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
					class="dark:bg-surface1 dark:border-surface3 dark:hover:bg-surface2 bg-background flex items-center justify-between gap-2 rounded-lg border border-transparent p-2 pl-4 shadow-sm hover:bg-gray-50"
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
						{#if catalogEntryServerId}
							<span class="text-on-surface1 text-sm">({catalogEntryServerId})</span>
						{/if}
					</div>
					<div class="icon-button">
						<ChevronRight class="size-6" />
					</div>
				</button>
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
			{@const auditLogsUrl = getAuditLogUrl(d)}
			{#if auditLogsUrl}
				<a href={auditLogsUrl} class="button-text"> View Audit Logs </a>
			{/if}
		{/snippet}
	</Table>
</div>
