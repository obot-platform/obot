<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CatalogServerForm from '$lib/components/admin/CatalogServerForm.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import {
		ChevronLeft,
		Container,
		Eye,
		LoaderCircle,
		Plus,
		Server,
		Trash2,
		User,
		Users,
		X
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	let loading = $state(true);
	let entries = $state<MCPCatalogEntry[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);

	onMount(async () => {
		entries = await ChatService.listMCPs();
		servers = await ChatService.listMCPCatalogServers();
		loading = false;
	});

	function convertEntriesToTableData(entries: MCPCatalogEntry[] | undefined) {
		if (!entries) {
			return [];
		}

		return entries.map((entry) => {
			return {
				id: entry.id,
				source: entry.sourceURL || 'manual',
				name: entry.commandManifest?.name ?? entry.urlManifest?.name ?? '',
				data: entry,
				users: '-',
				editable: !entry.sourceURL,
				type: 'single'
			};
		});
	}

	function convertServersToTableData(servers: MCPCatalogServer[] | undefined) {
		if (!servers) {
			return [];
		}

		return servers
			.filter((server) => !server.catalogEntryID)
			.map((server) => {
				return {
					id: server.id,
					name: server.manifest.name ?? '',
					source: 'manual',
					type: server.manifest.url ? 'remote' : 'multi',
					data: server,
					users: '-',
					editable: true
				};
			});
	}

	function convertEntriesAndServersToTableData(
		entries: MCPCatalogEntry[],
		servers: MCPCatalogServer[]
	) {
		const entriesTableData = convertEntriesToTableData(entries);
		const serversTableData = convertServersToTableData(servers);
		return [...entriesTableData, ...serversTableData];
	}

	let totalCount = $derived(entries.length + servers.length);
	let tableData = $derived(convertEntriesAndServersToTableData(entries, servers));
	let serverToDelete = $state<(typeof tableData)[0]>();

	let editingSource = $state<{ index: number; value: string }>();
	let sourceDialog = $state<HTMLDialogElement>();
	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedServerType = $state<'single' | 'multi' | 'remote'>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();

	let showServerForm = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	function selectServerType(type: 'single' | 'multi' | 'remote') {
		selectedServerType = type;
		selectServerTypeDialog?.close();
		showServerForm = true;
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceDialog?.close();
	}
	/*
			<button
						class="button-small flex items-center gap-1 text-xs font-normal"
						onclick={async () => {
							refreshing = true;
							await AdminService.refreshMCPCatalog(config.id);
							loadingEntries = AdminService.listMCPCatalogEntries(config.id);
							refreshing = false;
						}}
					>
						{#if refreshing}
							<LoaderCircle class="size-4 animate-spin" /> Refreshing...
						{:else}
							<RefreshCcw class="size-4" />
							Refresh Catalog
						{/if}
					</button>
	*/
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="flex flex-col gap-8 py-4" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen(selectedEntryServer)}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex items-center justify-between">
			<h1 class="text-2xl font-semibold">MCP Servers</h1>
			{#if totalCount > 0}
				<DotDotDot class="button-primary text-sm">
					{#snippet icon()}
						<span class="flex items-center gap-1">
							<Plus class="size-4" /> Add MCP Server
						</span>
					{/snippet}
					<div class="default-dialog flex min-w-max flex-col p-2">
						<button
							class="menu-button"
							onclick={() => {
								selectServerTypeDialog?.open();
							}}
						>
							Add server
						</button>
						<button
							class="menu-button"
							onclick={() => {
								editingSource = {
									index: -1,
									value: ''
								};
								sourceDialog?.showModal();
							}}
						>
							Add server(s) from Git
						</button>
					</div>
				</DotDotDot>
			{/if}
		</div>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else if totalCount === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Server class="size-24 text-gray-200 dark:text-gray-900" />
				<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
					No created MCP servers
				</h4>
				<p class="text-sm font-light text-gray-400 dark:text-gray-600">
					Looks like you don't have any servers created yet. <br />
					Click the button below to get started.
				</p>

				<button class="button-primary w-fit text-sm" onclick={() => (showServerForm = true)}
					>Add New Server</button
				>
			</div>
		{:else}
			<Table
				data={tableData}
				fields={['name', 'type', 'users', 'source']}
				onSelectRow={(d) => {
					selectedEntryServer = d.data;
					showServerForm = true;
				}}
				noDataMessage={'No catalog servers added.'}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'name'}
						<p class="flex items-center gap-1">
							{d.name}
							{#if d.source !== 'manual'}
								<span class="text-xs text-gray-500">({d.source.split('/').pop()})</span>{/if}
						</p>
					{:else if property === 'type'}
						{d.type === 'single' ? 'Single User' : d.type === 'multi' ? 'Multi-User' : 'Remote'}
					{:else if property === 'source'}
						{d.source === 'manual' ? 'Web Console' : d.source}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					{#if d.editable}
						<button
							class="icon-button hover:text-red-500"
							onclick={(e) => {
								e.stopPropagation();
								if (d.data.type === 'mcpserver') {
									deletingServer = d.data as MCPCatalogServer;
								} else {
									deletingEntry = d.data as MCPCatalogEntry;
								}
							}}
							use:tooltip={'Delete Entry'}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
					<button class="icon-button hover:text-blue-500" use:tooltip={'View Entry'}>
						<Eye class="size-4" />
					</button>
				{/snippet}
			</Table>
		{/if}
	</div>
{/snippet}

{#snippet configureEntryScreen(entry?: typeof selectedEntryServer)}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<button
			onclick={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
		>
			<ChevronLeft class="size-6" />
			Back to MCP Servers
		</button>

		<McpServerEntryForm
			{entry}
			type={selectedServerType}
			readonly={entry && 'sourceURL' in entry && !!entry.sourceURL}
			catalogId="default"
			onCancel={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			onSubmit={async () => {
				entries = await ChatService.listMCPs();
				servers = await ChatService.listMCPCatalogServers();
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
		/>
	</div>
{/snippet}

<dialog
	bind:this={sourceDialog}
	use:clickOutside={() => closeSourceDialog()}
	class="w-full max-w-md p-4"
>
	{#if editingSource}
		<h3 class="default-dialog-title">
			{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
			<button onclick={() => closeSourceDialog()} class="icon-button">
				<X class="size-5" />
			</button>
		</h3>

		<div class="my-4 flex flex-col gap-1">
			<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize"
				>Source URL
			</label>
			<input id="catalog-source-name" bind:value={editingSource.value} class="text-input-filled" />
		</div>

		<div class="flex w-full justify-end gap-2">
			<button class="button" onclick={() => closeSourceDialog()}>Cancel</button>
			<button
				class="button-primary"
				onclick={async () => {
					if (!editingSource) {
						return;
					}

					// saving = true;
					// if (editingSource.index === -1) {
					// 	mcpCatalog.sourceURLs = [...(mcpCatalog.sourceURLs ?? []), editingSource.value];
					// } else {
					// 	mcpCatalog.sourceURLs[editingSource.index] = editingSource.value;
					// }

					// if (mcpCatalog.id) {
					// 	const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
					// 	mcpCatalog = response;
					// }
					// saving = false;
					closeSourceDialog();
				}}
			>
				Add
			</button>
		</div>
	{/if}
</dialog>

<!-- <Confirm
	msg={`Are you sure you want to delete this catalog entry?`}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry) {
			return;
		}
		saving = true;
		await AdminService.deleteMCPCatalogEntry(mcpCatalog.id, deletingEntry.id);
		loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
		deletingEntry = undefined;
		saving = false;
	}}
	oncancel={() => (deletingEntry = undefined)}
/> -->

<!-- <Confirm
	msg={`Are you sure you want to delete this catalog server?`}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) {
			return;
		}
		saving = true;
		await AdminService.deleteMCPCatalogServer(mcpCatalog.id, deletingServer.id);
		loadingServers = AdminService.listMCPCatalogServers(mcpCatalog.id);
		deletingServer = undefined;
		saving = false;
	}}
	oncancel={() => (deletingServer = undefined)}
/> -->

<ResponsiveDialog title="Select Server Type" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="my-4 flex flex-col gap-4">
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('single')}
		>
			<User
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Single User Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for servers that require individualized configuration or were
					not designed for multi-user access, such as most studio MCP servers. When a user selects
					this server, a private instance will be created for them.
				</span>
			</div>
		</button>
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('multi')}
		>
			<Users
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Multi-User Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for servers designed to handle multiple user connections, such
					as most Streamable HTTP servers. When you create this server, a running instance will be
					deployed and any user with access to this catlog will be able to connect to it.
				</span>
			</div>
		</button>
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('remote')}
		>
			<Container
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Remote Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for allowing users to connect to MCP servers that are already
					elsewhere. When a user selects this server, their connection to the remote MCP server will
					go through the Obot gateway.
				</span>
			</div>
		</button>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
