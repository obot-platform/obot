<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService, ChatService, type MCP, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import {
		CircleAlert,
		Ellipsis,
		GitCompare,
		LoaderCircle,
		Server,
		ServerCog,
		Square,
		SquareCheck,
		TriangleAlert
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import Search from '$lib/components/Search.svelte';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import DiffDialog from '$lib/components/DiffDialog.svelte';
	import Confirm from '$lib/components/Confirm.svelte';

	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
	let search = $state('');

	initMcpServerAndEntries();
	const mcpServerAndEntries = getAdminMcpServerAndEntries();
	let users = $state<OrgUser[]>([]);
	let loading = $state(false);
	let data = $state<
		{
			id: string;
			name?: string;
			icon?: string;
			parentServer?: string;
			registry?: string;
			user?: string;
			needsUpdate?: boolean;
			server?: MCPCatalogServer;
			entry?: MCPCatalogEntry;
		}[]
	>([]);

	let diffDialog = $state<ReturnType<typeof DiffDialog>>();
	let diffServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, MCPCatalogServer>>({});
	let updating = $state<Record<string, { inProgress: boolean; error: string }>>({});

	let existingServer = $state<MCPCatalogServer>();
	let updatedServer = $state<MCPCatalogServer | MCPCatalogEntry>();
	let showConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>();

	let hasSelected = $derived(Object.values(selected).some((v) => v));
	let numServerUpdatesNeeded = $derived(data.filter((d) => d.needsUpdate).length);

	function convertToTableItem(
		server: MCPCatalogServer,
		usersMap: Map<string, OrgUser>,
		entry?: MCPCatalogEntry
	) {
		const { manifest, powerUserWorkspaceID, catalogEntryID, id, needsUpdate } = server;
		return {
			id,
			powerUserWorkspaceID,
			catalogEntryID,
			name: server.alias || manifest.name,
			icon: server.manifest.icon || entry?.manifest.icon,
			parentServer: entry?.manifest.name,
			registry: entry?.powerUserWorkspaceID
				? getUserDisplayName(usersMap, entry.powerUserWorkspaceID)
				: undefined,
			user: server.userID ? getUserDisplayName(usersMap, server.userID) : undefined,
			needsUpdate,
			server,
			entry
		};
	}

	onMount(async () => {
		loading = true;
		await fetchMcpServerAndEntries(
			defaultCatalogId,
			mcpServerAndEntries,
			async (entries, servers) => {
				const deployedCatalogEntryServers =
					await AdminService.listAllCatalogDeployedSingleRemoteServers(defaultCatalogId);
				const deployedWorkspaceCatalogEntryServers =
					await AdminService.listAllWorkspaceDeployedSingleRemoteServers();
				users = await AdminService.listUsersIncludeDeleted();
				const usersMap = new Map(users.map((user) => [user.id, user]));

				const entryMap = new Map(entries.map((entry) => [entry.id, entry]));
				data = [
					...deployedCatalogEntryServers.map((server) =>
						convertToTableItem(server, usersMap, entryMap.get(server.catalogEntryID))
					),
					...deployedWorkspaceCatalogEntryServers.map((server) =>
						convertToTableItem(server, usersMap, entryMap.get(server.catalogEntryID))
					),
					...servers.map((server) => convertToTableItem(server, usersMap, undefined))
				];
				loading = false;
			}
		);
	});

	async function handleMultiUpdate() {
		// TODO: update
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

		// listEntryServers =
		// 	entity === 'workspace'
		// 		? ChatService.listWorkspaceMCPServersForEntry(id, entry.id)
		// 		: AdminService.listMCPServersForEntry(id, entry.id);
		selected = {};
	}

	async function updateServer(server?: MCPCatalogServer) {
		// TODO: update
		if (!server) return;
		updating[server.id] = { inProgress: true, error: '' };
		try {
			await ChatService.triggerMcpServerUpdate(server.id);
			// listEntryServers =
			// 	entity === 'workspace'
			// 		? ChatService.listWorkspaceMCPServersForEntry(id, entry.id)
			// 		: AdminService.listMCPServersForEntry(id, entry.id);
		} catch (err) {
			updating[server.id] = {
				inProgress: false,
				error: err instanceof Error ? err.message : 'An unknown error occurred'
			};
		}

		delete updating[server.id];
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="flex flex-col gap-8 pt-4 pb-8" in:fade>
		<div
			class="flex flex-col gap-4 md:gap-8"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<div class="flex flex-col items-center justify-start md:flex-row md:justify-between">
				<h1 class="flex w-full items-center gap-2 text-2xl font-semibold">Deployed Servers</h1>
			</div>

			<div class="flex flex-col gap-2">
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
				<Search
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={(val) => (search = val)}
					placeholder="Search servers..."
				/>

				{#if loading}
					<div class="my-2 flex items-center justify-center">
						<LoaderCircle class="size-6 animate-spin" />
					</div>
				{:else}
					<Table
						{data}
						fields={['name', 'parentServer', 'user', 'registry', 'needsUpdate']}
						filterable={['name', 'parentServer', 'user', 'registry', 'needsUpdate']}
						headers={[
							{ title: 'Parent Server', property: 'parentServer' },
							{ title: 'Needs Update', property: 'needsUpdate' }
						]}
						onSelectRow={(d, isCtrlClick) => {
							// TODO;
						}}
						noDataMessage="No servers deployed."
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'name'}
								<div class="flex items-center gap-2">
									{#if d.icon}
										<img src={d.icon} alt={d.icon} class="size-6" />
									{:else}
										<Server class="size-6" />
									{/if}
									{d.name}
								</div>
							{:else if property === 'needsUpdate'}
								{#if d.needsUpdate}
									<div class="flex grow items-center justify-center">
										<TriangleAlert class="size-4 text-yellow-500" />
									</div>
								{/if}
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
						{#snippet actions(d)}
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
												existingServer = d.server;
												updatedServer = d.entry;
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
												if (!d.server) return;
												showConfirm = {
													type: 'single',
													server: d.server
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
										if (!d.server) return;
										if (selected[d.id]) {
											delete selected[d.id];
										} else {
											selected[d.id] = d.server;
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
						{/snippet}
					</Table>
				{/if}
			</div>
		</div>
	</div>
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
</Layout>

<DiffDialog bind:this={diffDialog} fromServer={existingServer} toServer={updatedServer} />

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

<svelte:head>
	<title>Obot | Deployed Servers</title>
</svelte:head>
