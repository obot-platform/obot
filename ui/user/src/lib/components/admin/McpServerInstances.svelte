<script lang="ts">
	import {
		AdminService,
		ChatService,
		Role,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';

	import {
		CircleAlert,
		Ellipsis,
		GitCompare,
		LoaderCircle,
		Router,
		ServerCog,
		Square,
		SquareCheck,
		TriangleAlert
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
	import DiffDialog from '../DiffDialog.svelte';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		users?: OrgUser[];
		type?: 'single' | 'multi' | 'remote';
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
	let isAdminUrl = $state(false);

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

		if (location.pathname.includes('/admin')) {
			isAdminUrl = true;
		}
	});

	async function handleMultiUpdate() {
		if (!id || !entry) return;
		for (const id of Object.keys(selected)) {
			updating[id] = { inProgress: true, error: '' };
			try {
				await ChatService.triggerMcpServerUpdate(id);
				updating[id] = { inProgress: false, error: '' };
			} catch (error) {
				updating[id] = {
					inProgress: false,
					error: error instanceof Error ? error.message : 'An unknown error occurred'
				};
			} finally {
				delete updating[id];
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
			await ChatService.triggerMcpServerUpdate(server.id);
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
				<div class="group mb-2 w-fit rounded-md bg-white dark:bg-black">
					<div
						class="flex items-center gap-1 rounded-md border border-yellow-500 bg-yellow-500/10 px-4 py-2 transition-colors duration-300 group-hover:bg-yellow-500/20 dark:bg-yellow-500/30 dark:group-hover:bg-yellow-500/40"
					>
						<TriangleAlert class="size-4 text-yellow-500" />
						<p class="text-sm font-light text-yellow-500">
							{#if numServerUpdatesNeeded === 1}
								1 instance has an update available.
							{:else}
								{numServerUpdatesNeeded} instances have updates available.
							{/if}
						</p>
					</div>
				</div>
			{/if}
			<Table
				data={servers}
				fields={type === 'single' ? ['userID', 'created'] : ['url', 'userID', 'created']}
				headers={[
					{ title: 'User', property: 'userID' },
					{ title: 'URL', property: 'url' }
				]}
				onSelectRow={type === 'single'
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
									<TriangleAlert class="size-4 text-yellow-500" />
								</div>
							{/if}
						</span>
					{:else if property === 'userID'}
						{@const user = usersMap.get(d[property] as string)}
						<span class="flex items-center gap-1">
							{user?.email || user?.username || 'Unknown'}
							{#if type === 'single'}
								{#if d.needsUpdate}
									<div
										use:tooltip={{
											text: 'This server needs an update. View Diff to see the changes.',
											classes: ['break-words', 'w-58']
										}}
									>
										<TriangleAlert class="size-4 text-yellow-500" />
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
					{@const url =
						entity === 'workspace'
							? `/admin/mcp-servers/w/${id}/c/${entry?.id}?view=audit-logs&mcp_id=${d.id}`
							: `/admin/mcp-servers/c/${entry?.id}?view=audit-logs&mcp_id=${d.id}`}
					<div class="flex items-center gap-1">
						{#if profile.current?.role === Role.ADMIN && isAdminUrl}
							<a class="button-text" href={url}> View Audit Logs </a>
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
										class="menu-button bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/20"
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
											<ServerCog class="size-4" />
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
