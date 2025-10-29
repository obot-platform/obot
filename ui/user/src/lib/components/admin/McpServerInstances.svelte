<script lang="ts">
	import {
		AdminService,
		ChatService,
		Group,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';

	import {
		CircleAlert,
		CircleFadingArrowUp,
		Ellipsis,
		GitCompare,
		LoaderCircle,
		Router,
		Square,
		SquareCheck
	} from 'lucide-svelte';
	import { formatTimeAgo } from '$lib/time';
	import { profile } from '$lib/stores';
	import DotDotDot from '../DotDotDot.svelte';
	import { onMount } from 'svelte';
	import Table from '../table/Table.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '../Confirm.svelte';
	import McpServerK8sInfo from './McpServerK8sInfo.svelte';
	import { openUrl } from '$lib/utils';
	import DiffDialog from './DiffDialog.svelte';
	import { page } from '$app/state';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		users?: OrgUser[];
		type?: 'single' | 'multi' | 'remote' | 'composite';
	}

	let { id, entity = 'catalog', entry, users = [], type }: Props = $props();

	let listServerInstances = $state<Promise<MCPServerInstance[]>>();
	let listEntryServers = $state<Promise<MCPCatalogServer[]>>();

	let showConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>();
	let diffDialog = $state<ReturnType<typeof DiffDialog>>();
	let diffServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, MCPCatalogServer>>({});
	let updating = $state<Record<string, { inProgress: boolean; error: string }>>({});

	let hasSelected = $derived(Object.values(selected).some((v) => v));
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));

	onMount(() => {
		if (entry && !('isCatalogEntry' in entry) && id) {
			if (entity === 'workspace') {
				listServerInstances = ChatService.listWorkspaceMcpCatalogServerInstances(id, entry.id);
			} else {
				listServerInstances = AdminService.listMcpCatalogServerInstances(id, entry.id);
			}
		} else if (entry && 'isCatalogEntry' in entry && id) {
			if (entity === 'workspace') {
				listEntryServers = ChatService.listWorkspaceMCPServersForEntry(id, entry.id);
			} else {
				listEntryServers = AdminService.listMCPServersForEntry(id, entry.id);
			}
		}
	});

	async function handleMultiUpdate() {
		if (!id || !entry) return;
		for (const serverId of Object.keys(selected)) {
			updating[serverId] = { inProgress: true, error: '' };
			try {
				await (entity === 'workspace' && id && entry
					? ChatService.triggerWorkspaceMcpServerUpdate(id, entry.id, serverId)
					: ChatService.triggerMcpServerUpdate(serverId));
				updating[serverId] = { inProgress: false, error: '' };
			} catch (error) {
				updating[serverId] = {
					inProgress: false,
					error: error instanceof Error ? error.message : 'An unknown error occurred'
				};
			} finally {
				delete updating[serverId];
			}
		}

		listEntryServers =
			entity === 'workspace'
				? ChatService.listWorkspaceMCPServersForEntry(id, entry.id)
				: AdminService.listMCPServersForEntry(id, entry.id);
		selected = {};
	}

	async function updateServer(server?: MCPCatalogServer) {
		if (!id || !entry || !server) return;

		updating[server.id] = { inProgress: true, error: '' };
		try {
			await (entity === 'workspace' && id && entry
				? ChatService.triggerWorkspaceMcpServerUpdate(id, entry.id, server.id)
				: ChatService.triggerMcpServerUpdate(server.id));
			listEntryServers =
				entity === 'workspace'
					? ChatService.listWorkspaceMCPServersForEntry(id, entry.id)
					: AdminService.listMCPServersForEntry(id, entry.id);
		} catch (err) {
			updating[server.id] = {
				inProgress: false,
				error: err instanceof Error ? err.message : 'An unknown error occurred'
			};
		}

		delete updating[server.id];
	}

	function setLastVisitedMcpServer() {
		if (!entry) return;
		const name = entry.manifest?.name;
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({ id: entry.id, name, type, entity, entityId: id })
		);
	}

	function getAuditLogUrl(d: MCPCatalogServer) {
		if (isAdminUrl) {
			if (!profile.current?.hasAdminAccess?.()) return null;
			return entity === 'workspace'
				? `/admin/mcp-servers/w/${id}/c/${entry?.id}?view=audit-logs&mcp_id=${d.id}&user_id=${d.userID}`
				: `/admin/mcp-servers/c/${entry?.id}?view=audit-logs&mcp_id=${d.id}&user_id=${d.userID}`;
		}

		return profile.current?.groups.includes(Group.POWERUSER)
			? `/mcp-publisher/c/${entry?.id}?view=audit-logs&mcp_id=${d.id}&user_id=${d.userID}`
			: null;
	}
</script>

{#if listServerInstances}
	{#await listServerInstances}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then instances}
		{#if entry && (type === 'multi' || instances.length > 0)}
			<div class="flex flex-col gap-6">
				<McpServerK8sInfo
					{id}
					{entity}
					mcpServerId={entry.id}
					name={'manifest' in entry ? entry.manifest.name || '' : ''}
					connectedUsers={instances.map((instance) => {
						const user = usersMap.get(instance.userID)!;
						return {
							...user,
							mcpInstanceId: instance.id
						};
					})}
					title="Details"
					classes={{
						title: 'text-lg font-semibold'
					}}
					readonly={profile.current.isAdminReadonly?.()}
				/>
			</div>
		{:else}
			{@render emptyInstancesContent()}
		{/if}
	{/await}
{:else if listEntryServers}
	{#await listEntryServers}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then servers}
		{@const numServerUpdatesNeeded = servers.filter((s) => s.needsUpdate).length}
		{#if servers.length > 0}
			{#if numServerUpdatesNeeded}
				<button
					class="group mb-2 w-fit rounded-md bg-white dark:bg-black"
					onclick={() => {
						// TODO: show all servers with upgrade & update all option
					}}
				>
					<div
						class="flex items-center gap-1 rounded-md border border-blue-500 bg-blue-500/10 px-4 py-2 transition-colors duration-300 group-hover:bg-blue-500/20 dark:bg-blue-500/30 dark:group-hover:bg-blue-500/40"
					>
						<CircleFadingArrowUp class="size-4 text-blue-500" />
						<p class="text-sm font-light text-blue-500">
							{#if numServerUpdatesNeeded === 1}
								1 instance has an update available.
							{:else}
								{numServerUpdatesNeeded} instances have updates available.
							{/if}
						</p>
					</div>
				</button>
			{/if}
			<Table
				data={servers}
				fields={type === 'single' ? ['userID', 'created'] : ['url', 'userID', 'created']}
				headers={[
					{ title: 'User', property: 'userID' },
					{ title: 'URL', property: 'url' }
				]}
				onClickRow={type === 'single'
					? (d, isCtrlClick) => {
							setLastVisitedMcpServer();

							const url =
								entity === 'workspace'
									? isAdminUrl
										? `/admin/mcp-servers/w/${id}/c/${entry?.id}/instance/${d.id}`
										: `/mcp-publisher/c/${entry?.id}/instance/${d.id}`
									: `/admin/mcp-servers/c/${entry?.id}/instance/${d.id}`;
							openUrl(url, isCtrlClick);
						}
					: undefined}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'url'}
						<span class="flex items-center gap-1">
							{d.manifest.remoteConfig?.url}
							{#if d.needsUpdate}
								<div
									use:tooltip={{
										text: 'This server needs an update. View Diff to see the changes.',
										classes: ['break-words', 'w-58']
									}}
								>
									<CircleFadingArrowUp class="size-4 text-blue-500" />
								</div>
							{/if}
						</span>
					{:else if property === 'userID'}
						{@const user = usersMap.get(d[property] as string)}
						<span class="flex items-center gap-1">
							{#if users.length === 0}
								<!--This covers the case where a Power User is listing their own servers.-->
								{profile.current.email || 'Unknown'}
							{:else}
								{user?.email || user?.username || 'Unknown'}
							{/if}
							{#if type === 'single'}
								{#if d.needsUpdate}
									<div
										use:tooltip={{
											text: 'This server needs an update. View Diff to see the changes.',
											classes: ['break-words', 'w-58']
										}}
									>
										<CircleFadingArrowUp class="size-4 text-blue-500" />
									</div>
								{/if}
							{/if}
						</span>
					{:else if property === 'created'}
						{formatTimeAgo(d[property] as unknown as string).fullDate}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}

				{#snippet actions(d)}
					{@const auditLogsUrl = getAuditLogUrl(d)}
					<div class="flex items-center gap-1">
						{#if auditLogsUrl}
							<a class="button-text" href={auditLogsUrl}> View Audit Logs </a>
						{/if}

						{#if d.needsUpdate}
							<DotDotDot class="icon-button hover:dark:bg-black/50">
								{#snippet icon()}
									<Ellipsis class="size-4" />
								{/snippet}

								<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
									<button
										class="menu-button"
										onclick={(e) => {
											e.stopPropagation();
											diffServer = d;
											diffDialog?.open();
										}}
									>
										<GitCompare class="size-4" /> View Diff
									</button>
									<button
										class="menu-button bg-blue-500/10 text-blue-500 hover:bg-blue-500/20"
										disabled={updating[d.id]?.inProgress}
										onclick={async (e) => {
											e.stopPropagation();
											showConfirm = {
												type: 'single',
												server: d
											};
										}}
									>
										{#if updating[d.id]?.inProgress}
											<LoaderCircle class="size-4 animate-spin" />
										{:else}
											<CircleFadingArrowUp class="size-4" />
										{/if}
										Update Server
									</button>
								</div>
							</DotDotDot>
							<button
								class="icon-button hover:bg-black/50"
								onclick={(e) => {
									e.stopPropagation();
									if (selected[d.id]) {
										delete selected[d.id];
									} else {
										selected[d.id] = d;
									}
								}}
							>
								{#if selected[d.id]}
									<SquareCheck class="size-5" />
								{:else}
									<Square class="size-5" />
								{/if}
							</button>
						{:else if numServerUpdatesNeeded > 0}
							<div class="size-10"></div>
							<div class="size-10"></div>
						{/if}
					</div>
				{/snippet}
			</Table>

			{#if hasSelected}
				{@const numSelected = Object.keys(selected).length}
				{@const updatingInProgress = Object.values(updating).some((u) => u.inProgress)}
				<div
					class="bg-surface1 sticky bottom-0 left-0 mt-auto flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
				>
					<div class="flex w-full items-center justify-between">
						<p class="text-sm font-medium">
							{numSelected} server instance{numSelected === 1 ? '' : 's'} selected
						</p>
						<div class="flex items-center gap-4">
							<button
								class="button flex items-center gap-1"
								onclick={() => {
									selected = {};
									updating = {};
								}}
							>
								Cancel
							</button>
							<button
								class="button-primary flex items-center gap-1"
								onclick={() => {
									showConfirm = {
										type: 'multi'
									};
								}}
								disabled={updatingInProgress}
							>
								{#if updatingInProgress}
									<LoaderCircle class="size-5" />
								{:else}
									Update Servers
								{/if}
							</button>
						</div>
					</div>
				</div>
			{/if}
		{:else}
			{@render emptyInstancesContent()}
		{/if}
	{/await}
{:else}
	{@render emptyInstancesContent()}
{/if}

<DiffDialog bind:this={diffDialog} fromServer={diffServer} toServer={entry} />

{#snippet emptyInstancesContent()}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Router class="size-24 text-gray-200 dark:text-gray-900" />
		<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No server details</h4>
		<p class="text-sm font-light text-gray-400 dark:text-gray-600">
			No details available yet for this server.
		</p>
	</div>
{/snippet}

<Confirm
	show={!!showConfirm}
	onsuccess={async () => {
		if (!showConfirm) return;
		if (showConfirm.type === 'single') {
			await updateServer(showConfirm.server);
		} else {
			await handleMultiUpdate();
		}
		showConfirm = undefined;
	}}
	oncancel={() => (showConfirm = undefined)}
	classes={{
		confirm: 'bg-blue-500 hover:bg-blue-400 transition-colors duration-200'
	}}
>
	{#snippet title()}
		<h4 class="mb-4 flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${showConfirm?.type === 'single' ? showConfirm.server.id : 'selected server(s)'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="mb-8 text-sm font-light">
			If this update introduces new required configuration parameters, users will have to supply
			them before they can use {showConfirm?.type === 'multi' ? 'these servers' : 'this server'} again.
		</p>
	{/snippet}
</Confirm>
