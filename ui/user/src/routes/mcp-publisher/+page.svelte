<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { Eye, LoaderCircle, Plus, Server, Trash2 } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import BackLink from '$lib/components/BackLink.svelte';
	import Search from '$lib/components/Search.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { convertEntriesAndServersToTableData } from '$lib/services/chat/mcp.js';

	let { data } = $props();
	let search = $state('');
	let workspaceId = $derived(data.workspace?.id);

	initMcpServerAndEntries();
	const mcpServerAndEntries = getPoweruserWorkspace();

	onMount(async () => {
		if (workspaceId) {
			await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries, (entries, servers) => {
				const serverId = new URL(window.location.href).searchParams.get('id');
				if (serverId) {
					const foundEntry = entries.find((e) => e.id === serverId);
					const foundServer = servers.find((s) => s.id === serverId);
					const found = foundEntry || foundServer;
					if (found && selectedEntryServer?.id !== found.id) {
						selectedEntryServer = found;
						showServerForm = true;
					} else if (!found && selectedEntryServer) {
						selectedEntryServer = undefined;
						showServerForm = false;
					}
				} else {
					selectedEntryServer = undefined;
					showServerForm = false;
				}
			});
		}
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const serverId = to.url.searchParams.get('id');
			const createNewType = to.url.searchParams.get('new') as 'single' | 'multi' | 'remote';
			if (createNewType) {
				selectServerType(createNewType, false);
			} else if (!serverId && (selectedEntryServer || showServerForm)) {
				selectedEntryServer = undefined;
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

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let selectedServerType = $state<'single' | 'multi' | 'remote' | 'composite'>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();

	let showServerForm = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	function selectServerType(type: 'single' | 'multi' | 'remote' | 'composite', updateUrl = true) {
		selectedServerType = type;
		selectServerTypeDialog?.close();
		showServerForm = true;
		if (updateUrl) {
			goto(`/mcp-publisher?new=${type}`, { replaceState: false });
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout showUserLinks>
	<div class="flex flex-col gap-8 pt-4 pb-8" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col gap-4 md:gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex flex-col items-center justify-start md:flex-row md:justify-between">
			<h1 class="flex w-full items-center gap-2 text-2xl font-semibold">MCP Servers</h1>
			{#if totalCount > 0}
				<div class="mt-4 w-full flex-shrink-0 md:mt-0 md:w-fit">
					{@render addServerButton()}
				</div>
			{/if}
		</div>

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
						No created MCP servers
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Looks like you don't have any servers created yet. <br />
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
								? `/mcp-publisher/c/${d.id}`
								: `/mcp-publisher/s/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					sortable={['name', 'type', 'users', 'created']}
					noDataMessage="No catalog servers added."
					filterable={['name', 'type']}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							<div class="flex flex-shrink-0 items-center gap-2">
								<div
									class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
								>
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
							{d.type === 'single' ? 'Single User' : d.type === 'multi' ? 'Multi-User' : 'Remote'}
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
	{@const currentLabelType =
		selectedServerType === 'single'
			? 'Single User'
			: selectedServerType === 'multi'
				? 'Multi-User'
				: 'Remote'}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<BackLink fromURL="mcp-publisher" currentLabel={`Create ${currentLabelType} Server`} />
		<McpServerEntryForm
			type={selectedServerType}
			id={workspaceId}
			entity="workspace"
			onCancel={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			onSubmit={async (id, type) => {
				if (type === 'single' || type === 'remote') {
					goto(`/mcp-publisher/c/${id}`);
				} else {
					goto(`/mcp-publisher/s/${id}`);
				}
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<button
		class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={() => {
			selectServerTypeDialog?.open();
		}}
	>
		<Plus class="size-4" /> Add MCP Server
	</button>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this server?"
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
/>

<Confirm
	msg="Are you sure you want to delete this server?"
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
/>

<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />

<svelte:head>
	<title>Obot | MCP Publisher</title>
</svelte:head>
