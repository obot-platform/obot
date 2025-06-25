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
		type MCP,
		type MCPCatalogServer,
		type ProjectMCP
	} from '$lib/services/index.js';
	import { ChevronLeft, ChevronRight, LoaderCircle, Server, Trash2, Unplug } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { project } = data;

	let projectServers = $state<ProjectMCP[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCP[]>([]);
	let loading = $state(true);

	let deletingProjectMcp = $state<string>();
	let connectToEntry = $state<{
		matchingProject?: ProjectMCP;
		entry: MCP;
		envs: MCPServerInfo['env'];
		headers: MCPServerInfo['headers'];
		connectURL?: string;
		launching: boolean;
	}>();
	let connectToServer = $state<MCPCatalogServer>();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let search = $state('');
	let connectedProjects = $derived(new Map(projectServers?.map((s) => [s.catalogEntryID, s])));
	let allData = $derived([
		...(entries.map((e) => ({
			...e,
			connectedProject: connectedProjects.get(e.id)
		})) ?? []),
		...(servers ?? [])
	]);
	let filteredData = $derived(
		search
			? allData.filter((item) => {
					const nameToUse =
						'manifest' in item
							? item.manifest.name
							: (item.commandManifest?.name ?? item.urlManifest?.name);
					return nameToUse?.toLowerCase().includes(search.toLowerCase());
				})
			: allData
	);
	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

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

	function closeConfigDialog() {
		connectToServer = undefined;
		connectToEntry = undefined;
	}

	function parseCategories(item: MCP | MCPCatalogServer) {
		if ('manifest' in item && item.manifest.metadata?.categories) {
			return item.manifest.metadata.categories.split(',') ?? [];
		}
		if ('commandManifest' in item && item.commandManifest?.metadata?.categories) {
			return item.commandManifest.metadata.categories.split(',') ?? [];
		}
		if ('urlManifest' in item && item.urlManifest?.metadata.categories) {
			return item.urlManifest.metadata.categories.split(',') ?? [];
		}
		return [];
	}

	async function handleMcpServer(server: MCPCatalogServer) {
		connectToServer = server;
	}

	async function handleMcpEntry(entry: MCP, connectedProject?: ProjectMCP) {
		const envs = (
			(entry.commandManifest ? entry.commandManifest.env : entry.urlManifest?.env) ?? []
		).map((env) => ({ ...env, value: '' }));

		const headers = (
			(entry.commandManifest ? entry.commandManifest.headers : entry.urlManifest?.headers) ?? []
		).map((header) => ({ ...header, value: '' }));

		connectToEntry = { entry, matchingProject: connectedProject, envs, headers, launching: false };

		if (connectedProject) {
			connectToEntry.connectURL = connectedProject.connectURL;
		} else if (envs.length === 0) {
			handleLaunch();
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

			const mcpServerInfo: MCPServerInfo = {
				...serverManifest,
				env: connectToEntry.envs,
				headers: connectToEntry.headers
			};

			const response = await createProjectMcp(mcpServerInfo, project, connectToEntry.entry.id);
			projectServers = await reloadProjectServers(project.assistantID, project.id);
			connectToEntry.connectURL = response.connectURL;
			connectToEntry.launching = false;
		}
	}

	function handleSelectItem(item: (typeof paginatedData)[0]) {
		if (item.type === 'mcpserver') {
			handleMcpServer(item as MCPCatalogServer);
		} else {
			handleMcpEntry(item as MCP, connectedProjects.get(item.id));
		}
		configDialog?.open();
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
			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
				onChange={(val) => {
					search = val;
					page = 0;
				}}
				placeholder="Search by name..."
			/>
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
				{#each paginatedData as item}
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
								<span
									class="line-clamp-2 text-xs leading-4.5 font-light text-gray-400 dark:text-gray-600"
								>
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
							{#if 'connectedProject' in item && item.connectedProject}
								<DotDotDot
									class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
								>
									<div class="default-dialog flex min-w-max flex-col p-2">
										<button
											class="menu-button hover:text-blue-500"
											onclick={(e) => {
												e.stopPropagation();
												handleSelectItem(item);
											}}
										>
											<Unplug class="size-4" /> Connect
										</button>
										<button
											class="menu-button text-red-500"
											onclick={async (e) => {
												e.stopPropagation();
												if (!item.connectedProject) return;
												deletingProjectMcp = item.connectedProject.id;
											}}
										>
											<Trash2 class="size-4" /> Delete instance
										</button>
									</div>
								</DotDotDot>
							{:else}
								<button
									class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
									use:tooltip={'Connect to server'}
									onclick={(e) => {
										e.stopPropagation();
										handleSelectItem(item);
									}}
								>
									<Unplug class="size-4" />
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
			{#if filteredData.length > pageSize}
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
						{page + 1} of {Math.ceil(filteredData.length / pageSize)}
					</span>
					<button
						class="button-text flex items-center gap-1 disabled:no-underline disabled:opacity-50"
						onclick={() => (page = page + 1)}
						disabled={page === Math.floor(filteredData.length / pageSize)}
					>
						Next <ChevronRight class="size-4" />
					</button>
				</div>
			{:else}
				<div class="min-h-8 w-full"></div>
			{/if}
		{/if}
	</div>
</Layout>

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

<ResponsiveDialog bind:this={configDialog} onClose={closeConfigDialog}>
	{#snippet titleContent()}
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
				<img
					src={connectToServer.manifest.icon}
					alt={connectToServer.manifest.name}
					class="size-6"
				/>
			{:else}
				<Server class="size-6" />
			{/if}
			{connectToServer.manifest.name}
		{/if}
	{/snippet}

	{#if connectToEntry}
		{#if connectToEntry.launching}
			<div class="my-4 flex flex-col justify-center gap-4">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else if connectToEntry.connectURL}
			{@render connectUrlButton(connectToEntry.connectURL)}
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
	{:else if connectToServer}
		{@render connectUrlButton(connectToServer.connectURL)}
	{/if}
</ResponsiveDialog>

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
