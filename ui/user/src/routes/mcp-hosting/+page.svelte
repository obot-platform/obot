<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, type MCPCatalogServer, type MCPServerInstance } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { Eye, LoaderCircle, Plus, Server, Trash2 } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import Search from '$lib/components/Search.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte';
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType,
		parseCategories
	} from '$lib/services/chat/mcp.js';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import SelectServerTypeHosting from '$lib/components/mcp/SelectServerTypeHosting.svelte';
	import SearchMcpServers from '$lib/components/admin/SearchMcpServers.svelte';
	import MyMcpServers from '$lib/components/mcp/MyMcpServers.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { onMount } from 'svelte';
	import { profile } from '$lib/stores';

	let { data } = $props();
	let search = $state('');
	let workspaceId = $derived(data.workspace?.id);
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let entity = $derived<'catalog' | 'workspace'>(hasAdminAccess ? 'catalog' : 'workspace');

	initMcpServerAndEntries();
	const mcpServerAndEntries = getPoweruserWorkspace();

	onMount(async () => {
		if (workspaceId) {
			await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
		}
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const createNewType = to.url.searchParams.get('new') === 'true';
			if (createNewType) {
				showServerForm = true;
			} else {
				showServerForm = false;
			}
		}
	});

	let totalCount = $derived(
		mcpServerAndEntries.entries.length + mcpServerAndEntries.servers.length
	);

	let tableData = $derived(
		convertEntriesAndServersToTableData(mcpServerAndEntries.entries, mcpServerAndEntries.servers)
	);

	let filteredTableData = $derived(
		tableData
			.filter((d) => d.name.toLowerCase().includes(search.toLowerCase()))
			.sort((a, b) => {
				return a.name.localeCompare(b.name);
			})
	);

	let showServerForm = $state(false);
	let serverFormType = $state<'multi' | 'remote'>('multi');
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerTypeHosting>>();
	let addExistingMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let catalogGridDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	// For the catalog grid view
	let catalogServers = $state<MCPCatalogServer[]>([]);
	let catalogEntries = $state<MCPCatalogEntry[]>([]);
	let catalogLoading = $state(false);

	function handleAddServerClick() {
		selectServerTypeDialog?.open();
	}

	async function handleSelectServerType(type: 'single' | 'multi' | 'remote' | 'composite' | 'registry' | 'custom') {
		selectServerTypeDialog?.close();

		if (type === 'registry') {
			// Load catalog data and show grid view
			catalogLoading = true;
			catalogGridDialog?.open();
			try {
				const [entriesResult, serversResult] = await Promise.all([
					ChatService.listMCPs(),
					ChatService.listMCPCatalogServers()
				]);
				catalogEntries = entriesResult;
				catalogServers = serversResult;
			} catch (error) {
				console.error('Failed to load catalog:', error);
			} finally {
				catalogLoading = false;
			}
		} else if (type === 'custom') {
			// Launch custom server form with multi type
			serverFormType = 'multi';
			showServerForm = true;
			goto(`/mcp-hosting?new=true`, { replaceState: false });
		} else {
			// Handle remote, composite
			serverFormType = type as 'multi' | 'remote';
			showServerForm = true;
			goto(`/mcp-hosting?new=true`, { replaceState: false });
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(showServerForm ? `Add MCP Server` : 'MCP Servers');
</script>

<Layout {title} showBackButton={showServerForm}>
	<div class="flex flex-col gap-8 pb-8" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showServerForm}
			<div class="flex-shrink-0">
				{@render addServerButton()}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col gap-4 md:gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex flex-col gap-2">
			<Search
				class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
				onChange={(val) => (search = val)}
				placeholder="Search servers..."
			/>

			{#if mcpServerAndEntries.loading}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else if totalCount === 0}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Server class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No Hosted MCP servers
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Looks like you haven't hosted any MCP servers yet. <br />
						Click the button below to get started.
					</p>

					{@render addServerButton()}
				</div>
			{:else}
				<Table
					data={filteredTableData}
					fields={['name', 'type', 'users', 'created']}
					onClickRow={(d, isCtrlClick) => {
						const url =
							d.type === 'single' || d.type === 'remote'
								? `/mcp-hosting/c/${d.id}`
								: `/mcp-hosting/s/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					sortable={['name', 'type', 'users', 'created']}
					noDataMessage="No catalog servers added."
					filterable={['name', 'type']}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							<div class="flex flex-shrink-0 items-center gap-2">
								<div class="icon">
									{#if d.icon}
										<img src={d.icon} alt={d.name} class="size-6" />
									{:else}
										<Server class="size-6" />
									{/if}
								</div>
								<p class="flex items-center gap-1">
									{d.name}
								</p>
							</div>
						{:else if property === 'type'}
							{getServerTypeLabelByType(d.type)}
						{:else if property === 'created'}
							{formatTimeAgo(d.created).relativeTime}
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
	</div>
{/snippet}

{#snippet configureEntryScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpServerEntryForm
			type={serverFormType}
			id={workspaceId}
			{entity}
			onCancel={() => {
				goto('/mcp-hosting');
			}}
			onSubmit={async (id) => {
				// Multi-user servers use /s/, remote servers use /c/
				const url = serverFormType === 'multi' ? `/mcp-hosting/s/${id}` : `/mcp-hosting/c/${id}`;
				goto(url);
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<button
		class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={handleAddServerClick}
	>
		<Plus class="size-4" /> Add MCP Server
	</button>
{/snippet}

<McpConfirmDelete
	names={[deletingEntry?.manifest?.name ?? '']}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry || !workspaceId) {
			return;
		}

		await ChatService.deleteWorkspaceMCPCatalogEntry(workspaceId, deletingEntry.id);
		await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
		deletingEntry = undefined;
	}}
	oncancel={() => (deletingEntry = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={[deletingServer?.manifest?.name ?? '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer || !workspaceId) {
			return;
		}

		await ChatService.deleteWorkspaceMCPCatalogServer(workspaceId, deletingServer.id);
		await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
		deletingServer = undefined;
	}}
	oncancel={() => (deletingServer = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<SelectServerTypeHosting bind:this={selectServerTypeDialog} onSelectServerType={handleSelectServerType} {entity} />

<SearchMcpServers
	bind:this={addExistingMcpServerDialog}
	type="acr"
	exclude={[]}
	onAdd={async (mcpCatalogEntryIds, mcpServerIds) => {
		// Add selected servers to workspace
		if (!workspaceId) return;

		// Refresh the server list after adding
		await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
	}}
	mcpEntriesContextFn={getPoweruserWorkspace}
	{entity}
	workspaceId={workspaceId}
/>

<ResponsiveDialog
	bind:this={catalogGridDialog}
	title="Add From Registry"
	class="h-full w-full md:h-[80vh] md:max-w-6xl"
>
	<div class="h-full overflow-y-auto">
		<MyMcpServers
			userServerInstances={[]}
			userConfiguredServers={[]}
			servers={catalogServers.map(s => ({ ...s, categories: parseCategories(s) }))}
			entries={catalogEntries.map(e => ({ ...e, categories: parseCategories(e) }))}
			loading={catalogLoading}
			onConnectServer={async (connectedServer) => {
				// Handle adding the server
				if (!workspaceId) return;
				catalogGridDialog?.close();
				await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
			}}
			connectSelectText="Add"
		/>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | MCP Publisher</title>
</svelte:head>
