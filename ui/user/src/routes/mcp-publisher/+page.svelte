<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, Role, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import {
		AlertTriangle,
		Container,
		Eye,
		LoaderCircle,
		Plus,
		Server,
		Trash2,
		User,
		Users
	} from 'lucide-svelte';
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
	import { profile } from '$lib/stores/index.js';

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

	function convertEntriesToTableData(entries: MCPCatalogEntry[] | undefined) {
		if (!entries) {
			return [];
		}

		return entries
			.filter((entry) => !entry.deleted)
			.map((entry) => {
				return {
					id: entry.id,
					name: entry.manifest?.name ?? '',
					icon: entry.manifest?.icon,
					source: entry.sourceURL || 'manual',
					data: entry,
					users: entry.userCount ?? 0,
					editable: !entry.sourceURL,
					type: entry.manifest.runtime === 'remote' ? 'remote' : 'single',
					created: entry.created
				};
			});
	}

	function convertServersToTableData(servers: MCPCatalogServer[] | undefined) {
		if (!servers) {
			return [];
		}

		return servers
			.filter((server) => !server.catalogEntryID && !server.deleted)
			.map((server) => {
				return {
					id: server.id,
					name: server.manifest.name ?? '',
					icon: server.manifest.icon,
					source: 'manual',
					type: 'multi',
					data: server,
					users: server.mcpServerInstanceUserCount ?? 0,
					editable: true,
					created: server.created
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

	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedServerType = $state<'single' | 'multi' | 'remote'>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();

	let syncError = $state<{ url: string; error: string }>();
	let syncErrorDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let showServerForm = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	function selectServerType(type: 'single' | 'multi' | 'remote', updateUrl = true) {
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
					fields={['name', 'type', 'users', 'source', 'created']}
					onSelectRow={(d, isCtrlClick) => {
						const url =
							d.type === 'single' || d.type === 'remote'
								? `/mcp-publisher/c/${d.id}`
								: `/mcp-publisher/s/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					sortable={['name', 'type', 'users', 'source', 'created']}
					noDataMessage="No catalog servers added."
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
						{:else if property === 'source'}
							{d.source === 'manual' ? 'Web Console' : d.source}
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

		await ChatService.deleteWorkspaceMCPCatalogEntry(workspaceId, deletingServer.id);
		await fetchMcpServerAndEntries(workspaceId, mcpServerAndEntries);
		deletingServer = undefined;
	}}
	oncancel={() => (deletingServer = undefined)}
/>

<ResponsiveDialog title="Select Server Type" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="my-4 flex flex-col gap-4">
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
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
		{#if profile.current?.role === Role.POWERUSER_PLUS || profile.current?.role === Role.ADMIN}
			<button
				class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
				onclick={() => selectServerType('multi')}
			>
				<Users
					class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
				/>
				<div>
					<p class="mb-1 text-sm font-semibold">Multi-User Server</p>
					<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
						This option is appropriate for servers designed to handle multiple user connections,
						such as most Streamable HTTP servers. When you create this server, a running instance
						will be deployed and any user with access to this catlog will be able to connect to it.
					</span>
				</div>
			</button>
		{/if}
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
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

<ResponsiveDialog title="Git Source URL Sync" bind:this={syncErrorDialog} class="md:w-2xl">
	<div class="mb-4 flex flex-col gap-4">
		<div class="notification-alert flex flex-col gap-2">
			<div class="flex items-center gap-2">
				<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">
					An issue occurred fetching this source URL:
				</p>
			</div>
			<span class="text-sm font-light break-all">{syncError?.error}</span>
		</div>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | MCP Publisher</title>
</svelte:head>
