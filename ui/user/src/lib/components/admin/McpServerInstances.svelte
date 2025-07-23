<script lang="ts">
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';

	import {
		Ellipsis,
		GitCompare,
		LoaderCircle,
		Router,
		Server,
		ServerCog,
		Square,
		SquareCheck,
		TriangleAlert
	} from 'lucide-svelte';
	import { formatTimeAgo } from '$lib/time';
	import { responsive } from '$lib/stores';
	import { formatJsonWithDiffHighlighting, generateJsonDiff } from '$lib/diff';
	import DotDotDot from '../DotDotDot.svelte';
	import { onMount } from 'svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Table from '../Table.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { goto } from '$app/navigation';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		users?: OrgUser[];
		type?: 'single' | 'multi' | 'remote';
	}

	let { catalogId, entry, users = [], type }: Props = $props();

	let listServerInstances = $state<Promise<MCPServerInstance[]>>();
	let listEntryServers = $state<Promise<MCPCatalogServer[]>>();

	let diffDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let diffServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, MCPCatalogServer>>({});
	let updating = $state<Record<string, { inProgress: boolean; error: string }>>({});

	let hasSelected = $derived(Object.values(selected).some((v) => v));
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(() => {
		if (entry && 'manifest' in entry && catalogId) {
			listServerInstances = AdminService.listMcpCatalogServerInstances(catalogId, entry.id);
		} else if (entry && !('manifest' in entry) && catalogId) {
			listEntryServers = AdminService.listMCPServersForEntry(catalogId, entry.id);
		}
	});

	async function handleMultiUpdate() {
		if (!catalogId || !entry) return;
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

		listEntryServers = AdminService.listMCPServersForEntry(catalogId, entry.id);
		selected = {};
	}

	async function updateServer(server?: MCPCatalogServer) {
		if (!catalogId || !entry || !server) return;

		updating[server.id] = { inProgress: true, error: '' };
		try {
			await ChatService.triggerMcpServerUpdate(server.id);
			listEntryServers = AdminService.listMCPServersForEntry(catalogId, entry.id);
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
		const name =
			'manifest' in entry
				? entry.manifest?.name
				: (entry.commandManifest?.name ?? entry.urlManifest?.name);
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({ id: entry.id, name, type })
		);
	}
</script>

{#if listServerInstances}
	{#await listServerInstances}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then instances}
		{#if instances.length > 0}
			<Table
				data={instances}
				fields={['id', 'userID', 'created']}
				headers={[{ title: 'User', property: 'userID' }]}
				onSelectRow={(d) => {
					setLastVisitedMcpServer();
					goto(`/v2/admin/mcp-servers/s/${entry?.id}/instance/${d.id}`);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'userID'}
						{@const user = usersMap.get(d[property] as string)}
						{user?.email || user?.username || 'Unknown'}
					{:else if property === 'created'}
						{formatTimeAgo(d[property] as unknown as string).fullDate}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}

				{#snippet actions(d)}
					<button
						class="button-text"
						onclick={(e) => {
							e.stopPropagation();
							goto(`/v2/admin/audit-logs?mcpId=${encodeURIComponent(d.id)}`);
						}}
					>
						View Audit Logs
					</button>
				{/snippet}
			</Table>
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
				</button>
			{/if}
			<Table
				data={servers}
				fields={['id', 'created']}
				onSelectRow={type === 'single'
					? (d) => {
							setLastVisitedMcpServer();
							goto(`/v2/admin/mcp-servers/c/${entry?.id}/instance/${d.id}`);
						}
					: undefined}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'id'}
						<span class="flex items-center gap-1">
							{d.id}
							{#if d.needsUpdate}
								<div
									use:tooltip={{
										text: 'This server needs an update. Click diff to see the changes.',
										classes: ['break-words', 'w-58']
									}}
								>
									<TriangleAlert class="size-4 text-yellow-500" />
								</div>
							{/if}
						</span>
					{:else if property === 'created'}
						{formatTimeAgo(d[property] as unknown as string).fullDate}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}

				{#snippet actions(d)}
					<div class="flex items-center gap-1">
						<button
							class="button-text px-1"
							onclick={(e) => {
								e.stopPropagation();
								goto(`/v2/admin/audit-logs?mcpId=${encodeURIComponent(d.id)}`);
							}}
						>
							View Audit Logs
						</button>

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
											await updateServer(d);
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
								onclick={handleMultiUpdate}
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

<ResponsiveDialog bind:this={diffDialog} class="h-screen w-full p-0 md:w-[calc(100vw-2em)]">
	{#snippet titleContent()}
		{#if diffServer?.manifest}
			<div class="flex items-center gap-2 md:p-4 md:pb-0">
				<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
					{#if diffServer?.manifest?.icon}
						<img src={diffServer.manifest.icon} alt={diffServer.manifest.name} class="size-5" />
					{:else}
						<Server class="size-5" />
					{/if}
				</div>
				{diffServer.manifest.name} | {diffServer.id}
			</div>
		{/if}
	{/snippet}
	{#if entry}
		{@const newServerManifest =
			'commandManifest' in entry || 'urlManifest' in entry
				? (entry.commandManifest ?? entry.urlManifest)
				: undefined}
		{@const diffManifest = diffServer?.manifest}
		{#if newServerManifest && diffManifest}
			{@const diff = generateJsonDiff(diffManifest, newServerManifest)}
			{#if !responsive.isMobile}
				<div class="grid h-full grid-cols-2">
					<div class="h-full">
						<h3 class="mb-2 px-4 text-sm font-semibold text-gray-600 dark:text-gray-400">
							Current Version
						</h3>
						<div
							class="default-scrollbar-thin dark:border-surface3 dark:bg-surface1 h-full overflow-x-auto border-r border-gray-200 bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(diffManifest, diff, true)}
							</div>
						</div>
					</div>
					<div class="h-full">
						<h3 class="mb-2 px-4 text-sm font-semibold text-gray-600 dark:text-gray-400">
							New Version
						</h3>
						<div
							class="default-scrollbar-thin dark:border-surface3 dark:bg-surface1 h-full overflow-x-auto bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(newServerManifest, diff, false)}
							</div>
						</div>
					</div>
				</div>
			{:else}
				<div class="h-full w-full pl-2">
					<h3 class="mb-2 text-sm font-semibold text-gray-600 dark:text-gray-400">Source Diff</h3>
					<div
						class="default-scrollbar-thin dark:bg-surface1 h-full overflow-auto rounded-sm bg-gray-50 pt-4"
					>
						{#each diff.unifiedLines as line, i (i)}
							{@const type = line.startsWith('+')
								? 'added'
								: line.startsWith('-')
									? 'removed'
									: 'unchanged'}
							{@const content = line.startsWith('+') || line.startsWith('-') ? line.slice(1) : line}
							{@const prefix = line.startsWith('+') ? '+' : line.startsWith('-') ? '-' : ' '}
							<div
								class={twMerge(
									'font-mono text-sm whitespace-pre',
									type === 'added'
										? 'bg-green-500/10 text-green-500 dark:bg-green-900/30'
										: type === 'removed'
											? 'bg-red-500/10 text-red-500'
											: 'text-gray-700 dark:text-gray-300'
								)}
							>
								{prefix}{content}
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{:else}
			<div class="flex items-center justify-center py-8">
				<p class="text-gray-500 dark:text-gray-400">
					Unable to compare manifests. Missing manifest data.
				</p>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>

{#snippet emptyInstancesContent()}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Router class="size-24 text-gray-200 dark:text-gray-900" />
		<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No server instance</h4>
		<p class="text-sm font-light text-gray-400 dark:text-gray-600">
			No server instances have been created yet for this server.
		</p>
	</div>
{/snippet}
