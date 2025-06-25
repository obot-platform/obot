<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { createProjectMcp, type MCPServerInfo } from '$lib/services/chat/mcp';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServer,
		type ProjectMCP
	} from '$lib/services/index.js';
	import { ChevronLeft, ChevronRight, LoaderCircle, Server, Trash2, Unplug } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import McpServerEntry from '$lib/components/mcp/McpServerEntry.svelte';

	let { data } = $props();
	const { project } = data;

	let projectServers = $state<ProjectMCP[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);

	let deletingProjectMcp = $state<string>();
	let connectToEntry = $state<{
		matchingProject?: ProjectMCP;
		entry: MCPCatalogEntry;
		envs: MCPServerInfo['env'];
		headers: MCPServerInfo['headers'];
		connectURL?: string;
		launching: boolean;
	}>();
	let connectToServer = $state<MCPCatalogServer>();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let serverInfoDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let search = $state('');
	let connectedProjects = $derived(new Map(projectServers?.map((s) => [s.catalogEntryID, s])));
	let allEntries = $derived(
		entries.map((e) => ({
			...e,
			connectedProject: connectedProjects.get(e.id)
		})) ?? []
	);
	let filteredEntriesData = $derived(
		search
			? allEntries.filter((item) => {
					const nameToUse = item.commandManifest?.name ?? item.urlManifest?.name;
					return nameToUse?.toLowerCase().includes(search.toLowerCase());
				})
			: allEntries
	);
	let page = $state(0);
	let pageSize = $state(30);
	let paginatedEntriesData = $derived(
		filteredEntriesData.slice(page * pageSize, (page + 1) * pageSize)
	);
	let connectedServersData = $derived([
		...(servers ?? []),
		...allEntries.filter((e) => e.connectedProject)
	]);

	async function reloadProjectServers(assistantID: string, projectID: string) {
		const response = await ChatService.listProjectMCPs(assistantID, projectID);
		return response.filter((s) => !s.deleted);
	}

	async function loadData() {
		if (project) {
			loading = true;
			try {
				const [projectServersResult, entriesResult, serversResult] = await Promise.all([
					reloadProjectServers(project.assistantID, project.id),
					ChatService.listMCPs(),
					ChatService.listMCPCatalogServers()
				]);
				projectServers = projectServersResult.filter((s) => !s.deleted);
				entries = entriesResult;
				servers = serversResult;
			} catch (error) {
				console.error('Failed to load data:', error);
			} finally {
				loading = false;
			}
		}
	}

	onMount(() => {
		loadData();
	});

	function closeDialogs() {
		connectToServer = undefined;
		connectToEntry = undefined;
	}

	function parseCategories(item: (typeof connectedServersData)[0]) {
		if ('manifest' in item && item.manifest.metadata?.categories) {
			return item.manifest.metadata.categories.split(',') ?? [];
		}
		if ('commandManifest' in item && item.commandManifest?.metadata?.categories) {
			return item.commandManifest.metadata.categories.split(',') ?? [];
		}
		if ('urlManifest' in item && item.urlManifest?.metadata?.categories) {
			return item.urlManifest.metadata.categories.split(',') ?? [];
		}
		return [];
	}

	async function handleMcpServer(server: MCPCatalogServer) {
		connectToServer = server;
		connectDialog?.open();
	}

	async function handleMcpEntry(entry: MCPCatalogEntry, connectedProject?: ProjectMCP) {
		const envs = (
			(entry.commandManifest ? entry.commandManifest.env : entry.urlManifest?.env) ?? []
		).map((env) => ({ ...env, value: '' }));

		const headers = (
			(entry.commandManifest ? entry.commandManifest.headers : entry.urlManifest?.headers) ?? []
		).map((header) => ({ ...header, value: '' }));

		connectToEntry = { entry, matchingProject: connectedProject, envs, headers, launching: false };

		if (connectedProject) {
			connectToEntry.connectURL = connectedProject.connectURL;
			connectDialog?.open();
		} else {
			serverInfoDialog?.open();
		}
	}

	async function handleLaunch() {
		if (connectToEntry && project) {
			connectToEntry.launching = true;
			const serverManifest =
				connectToEntry.entry.commandManifest ?? connectToEntry.entry.urlManifest;

			if (!serverManifest) {
				console.error('No server manifest found');
				return;
			}

			const mcpServerInfo: MCPCatalogEntryServerManifest = {
				...serverManifest,
				env: connectToEntry.envs,
				headers: connectToEntry.headers
			};

			const response = await createProjectMcp(mcpServerInfo, project, connectToEntry.entry.id);
			projectServers = await reloadProjectServers(project.assistantID, project.id);
			connectToEntry.connectURL = response.connectURL;
			connectToEntry.launching = false;

			configDialog?.close();
			connectDialog?.open();
		}
	}

	function handleSelectItem(item: (typeof connectedServersData)[0]) {
		if (item.type === 'mcpserver') {
			handleMcpServer(item as MCPCatalogServer);
		} else {
			handleMcpEntry(item as MCPCatalogEntry, connectedProjects.get(item.id));
		}
	}
</script>

<Layout>
	<div class="flex flex-col gap-8 pt-4" in:fade>
		<h1 class="text-2xl font-semibold">MCP Servers</h1>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div class="flex flex-col gap-4">
				<h2 class="text-lg font-semibold">Connected MCP Servers</h2>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
					{#each connectedServersData as item}
						{@render mcpServerCard(item)}
					{/each}
				</div>
			</div>
			<div class="flex flex-col gap-4">
				<h2 class="text-lg font-semibold">Available MCP Servers</h2>
				<Search
					class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
					onChange={(val) => {
						search = val;
						page = 0;
					}}
					placeholder="Search by name..."
				/>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
					{#each paginatedEntriesData as item}
						{@render mcpServerCard(item)}
					{/each}
				</div>
				{#if filteredEntriesData.length > pageSize}
					<div
						class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-center gap-4 p-2 md:w-[calc(100%+4em)] md:-translate-x-8 dark:bg-black"
					>
						<button
							class="button-text flex items-center gap-1 disabled:no-underline disabled:opacity-50"
							onclick={() => (page = page - 1)}
							disabled={page === 0}
						>
							<ChevronLeft class="size-4" /> Previous
						</button>
						<span class="text-sm text-gray-400 dark:text-gray-600">
							{page + 1} of {Math.ceil(filteredEntriesData.length / pageSize)}
						</span>
						<button
							class="button-text flex items-center gap-1 disabled:no-underline disabled:opacity-50"
							onclick={() => (page = page + 1)}
							disabled={page === Math.floor(filteredEntriesData.length / pageSize)}
						>
							Next <ChevronRight class="size-4" />
						</button>
					</div>
				{:else}
					<div class="min-h-8 w-full"></div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet mcpServerCard(item: (typeof connectedServersData)[0])}
	{@const icon =
		'manifest' in item
			? item.manifest.icon
			: (item.commandManifest?.icon ?? item.urlManifest?.icon)}
	{@const name =
		'manifest' in item
			? item.manifest.name
			: (item.commandManifest?.name ?? item.urlManifest?.name)}
	{@const categories = parseCategories(item)}
	<div
		class="dark:bg-surface1 dark:border-surface3 relative flex flex-col rounded-sm border border-transparent bg-white px-2 py-4 shadow-sm"
	>
		<div class="flex items-center gap-2 pr-6">
			<div
				class="flex size-8 flex-shrink-0 items-center justify-center self-start rounded-md bg-transparent p-0.5 dark:bg-gray-600"
			>
				{#if icon}
					<img src={icon} alt={name} />
				{:else}
					<Server />
				{/if}
			</div>
			<div class="flex flex-col">
				<p class="text-sm font-semibold">{name}</p>
				<span class="line-clamp-2 text-xs leading-4.5 font-light text-gray-400 dark:text-gray-600">
					{#if 'manifest' in item}
						{item.manifest.description}
					{:else}
						{item.commandManifest?.description ?? item.urlManifest?.description}
					{/if}
				</span>
			</div>
		</div>
		<div class="flex w-full flex-wrap gap-1 pt-2">
			{#each categories as category}
				<div
					class="border-surface3 rounded-full border px-1.5 py-0.5 text-[10px] font-light text-gray-400 dark:text-gray-600"
				>
					{category}
				</div>
			{/each}
		</div>
		<div
			class="absolute -top-2 right-0 flex h-full translate-y-2 flex-col justify-between gap-4 p-2"
		>
			{#if ('connectedProject' in item && item.connectedProject) || 'manifest' in item}
				<DotDotDot
					class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
				>
					<div class="default-dialog flex min-w-max flex-col p-2">
						<button
							class="menu-button hover:text-blue-500"
							onclick={(e) => {
								handleSelectItem(item);
							}}
						>
							<Unplug class="size-4" /> Connection URL
						</button>
						{#if 'connectedProject' in item && item.connectedProject}
							<button
								class="menu-button text-red-500"
								onclick={async () => {
									if ('connectedProject' in item && item.connectedProject) {
										deletingProjectMcp = item.connectedProject.id;
									}
								}}
							>
								Disconnect
							</button>
						{/if}
					</div>
				</DotDotDot>
			{:else}
				<button
					class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
					use:tooltip={'Connect to server'}
					onclick={(e) => {
						handleSelectItem(item);
					}}
				>
					<Unplug class="size-4" />
				</button>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet connectUrlButton(url: string)}
	<div class="mb-8 flex flex-col gap-1">
		<label for="connectURL" class="font-light">Connection URL</label>
		<div class="mock-input-btn flex w-full items-center justify-between gap-2 shadow-inner">
			<p>
				{url}
			</p>
			<CopyButton
				showTextLeft
				text={url}
				classes={{
					button: 'flex-shrink-0 flex items-center gap-1 text-xs font-light hover:text-blue-500'
				}}
			/>
		</div>
	</div>

	<HowToConnect {url} />
{/snippet}

<ResponsiveDialog bind:this={serverInfoDialog}>
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry}
		<McpServerEntry entry={connectToEntry.entry as MCPCatalogEntry} />
	{/if}

	{#if connectToServer}
		<McpServerEntry entry={connectToServer} />
	{/if}

	<div class="flex justify-end">
		<button
			class="button-primary"
			onclick={() => {
				serverInfoDialog?.close();
				if (connectToEntry && connectToEntry.envs && connectToEntry.envs.length === 0) {
					handleLaunch();
				} else {
					configDialog?.open();
				}
			}}>Connect</button
		>
	</div>
</ResponsiveDialog>

<ResponsiveDialog bind:this={configDialog} animate="slide">
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry}
		{#if connectToEntry.launching}
			<div class="my-4 flex flex-col justify-center gap-4"></div>
		{:else}
			<div class="my-4 flex flex-col gap-4">
				{#if connectToEntry.envs && connectToEntry.envs.length > 0}
					{#each connectToEntry.envs as env, i}
						<div class="flex flex-col gap-1">
							<span class="flex items-center gap-2">
								<label for={env.key}>
									{env.name}
									{#if !env.required}
										<span class="text-gray-400 dark:text-gray-600">(optional)</span>
									{/if}
								</label>
								<InfoTooltip text={env.description} />
							</span>
							{#if env.sensitive}
								<SensitiveInput name={env.name} bind:value={connectToEntry.envs[i].value} />
							{:else}
								<input
									type="text"
									id={env.key}
									bind:value={connectToEntry.envs[i].value}
									class="text-input-filled"
								/>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
			<div class="flex justify-end">
				<button class="button-primary" onclick={handleLaunch}>Launch</button>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry?.connectURL}
		{@render connectUrlButton(connectToEntry.connectURL)}
	{:else if connectToServer}
		{@render connectUrlButton(connectToServer.connectURL)}
	{/if}
</ResponsiveDialog>

{#snippet title()}
	{#if connectToEntry}
		{@const name =
			connectToEntry.entry.commandManifest?.name ?? connectToEntry.entry.urlManifest?.name}
		{@const icon =
			connectToEntry.entry.commandManifest?.icon ?? connectToEntry.entry.urlManifest?.icon}
		{#if icon}
			<img src={icon} alt={name} class="size-6" />
		{:else}
			<Server class="size-6" />
		{/if}
		{name}
	{:else if connectToServer}
		{#if connectToServer.manifest.icon}
			<img src={connectToServer.manifest.icon} alt={connectToServer.manifest.name} class="size-6" />
		{:else}
			<Server class="size-6" />
		{/if}
		{connectToServer.manifest.name}
	{/if}
{/snippet}

<Confirm
	msg={'Are you sure you want to delete this server?'}
	show={Boolean(deletingProjectMcp)}
	onsuccess={async () => {
		if (deletingProjectMcp && project) {
			await ChatService.deleteProjectMCP(project.assistantID, project.id, deletingProjectMcp);
			projectServers = await reloadProjectServers(project.assistantID, project.id);
			deletingProjectMcp = undefined;
		}
	}}
	oncancel={() => (deletingProjectMcp = undefined)}
/>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
