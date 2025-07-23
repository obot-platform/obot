<script lang="ts">
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { createProjectMcp, parseCategories } from '$lib/services/chat/mcp';
	import {
		ChatService,
		EditorService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services/index.js';
	import { ExternalLink, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import PageLoading from '$lib/components/PageLoading.svelte';
	import { afterNavigate } from '$app/navigation';
	import MyMcpServers from '$lib/components/mcp/MyMcpServers.svelte';
	import { responsive } from '$lib/stores';

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);
	let chatLoading = $state(false);

	let connectToServer = $state<{
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
		connectURL?: string;
		parent?: MCPCatalogEntry;
	}>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let showAllServersConfigDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedCategory = $state('');

	let convertedEntries: (MCPCatalogEntry & { categories: string[] })[] = $derived(
		entries.map((entry) => ({
			...entry,
			categories: parseCategories(entry)
		}))
	);
	let convertedServers: (MCPCatalogServer & { categories: string[] })[] = $derived(
		servers.map((server) => ({
			...server,
			categories: parseCategories(server)
		}))
	);
	let convertedUserConfiguredServers: (MCPCatalogServer & { categories: string[] })[] = $derived(
		userConfiguredServers.map((server) => ({
			...server,
			categories: parseCategories(server)
		}))
	);

	let categories = $derived([
		...new Set([
			...convertedEntries.flatMap((item) => item.categories),
			...convertedServers.flatMap((item) => item.categories)
		])
	]);

	async function loadData(partialRefresh?: boolean) {
		loading = true;
		try {
			if (partialRefresh) {
				const [singleOrRemoteUserServers, serverInstances] = await Promise.all([
					ChatService.listSingleOrRemoteMcpServers(),
					ChatService.listMcpServerInstances()
				]);

				userConfiguredServers = singleOrRemoteUserServers;
				userServerInstances = serverInstances;
			} else {
				const [singleOrRemoteUserServers, entriesResult, serversResult, serverInstances] =
					await Promise.all([
						ChatService.listSingleOrRemoteMcpServers(),
						ChatService.listMCPs(),
						ChatService.listMCPCatalogServers(),
						ChatService.listMcpServerInstances()
					]);

				userConfiguredServers = singleOrRemoteUserServers;
				entries = entriesResult;
				servers = serversResult;
				userServerInstances = serverInstances;
			}
		} catch (error) {
			console.error('Failed to load data:', error);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadData();
	});

	afterNavigate(() => {
		const url = new URL(window.location.href);
		selectedCategory = url.searchParams.get('category') ?? '';
	});

	async function handleSetupChat(connectedServer: typeof connectToServer) {
		if (!connectedServer || !connectedServer.server) return;
		chatLoading = true;

		const projects = await ChatService.listProjects();
		const name = [connectedServer.server?.manifest.name ?? '', connectedServer.server.id].join(
			' - '
		);
		const match = projects.items.find((project) => project.name === name);

		let project = match;
		if (!match) {
			// if no project match, create a new one w/ mcp server connected to it
			project = await EditorService.createObot({
				name: name
			});
		}

		const mcpId = connectedServer.instance
			? connectedServer.instance.id
			: connectedServer.server.id;
		if (
			project &&
			!(await ChatService.listProjectMCPs(project.assistantID, project.id)).find(
				(mcp) => mcp.mcpID === mcpId
			)
		) {
			await createProjectMcp(project, mcpId);
		}

		window.open(`/o/${project?.id}`, '_blank');
		chatLoading = false;
	}
</script>

<Layout showUserLinks hideSidebar>
	<div class="flex w-full">
		{#if !responsive.isMobile}
			<ul class="flex min-h-0 w-xs flex-shrink-0 grow flex-col p-4">
				<li>
					<button
						class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
						class:!border-blue-500={!selectedCategory}
						onclick={() => {
							selectedCategory = '';
						}}
					>
						Browse All
					</button>
				</li>
				{#each categories as category (category)}
					<li>
						<button
							class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
							class:!border-blue-500={category === selectedCategory}
							onclick={() => {
								selectedCategory = category;
							}}
						>
							{category}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex w-full flex-col gap-8 pt-4" in:fade>
			<MyMcpServers
				{userServerInstances}
				userConfiguredServers={convertedUserConfiguredServers}
				servers={convertedServers}
				entries={convertedEntries}
				{loading}
				onConnectServer={(connectedServer) => {
					loadData(true);
					connectToServer = connectedServer;
					connectDialog?.open();
				}}
				onSelectConnectedServer={(connectedServer) => {
					connectToServer = connectedServer;
					connectDialog?.open();
				}}
				onDisconnect={() => {
					loadData(true);
				}}
				connectSelectText="Connect To Server"
				onUpdateConfigure={() => {
					loadData(true);
				}}
				{selectedCategory}
			>
				{#snippet appendConnectedServerTitle()}
					<button class="button text-xs" onclick={() => showAllServersConfigDialog?.open()}>
						Generate Configuration
					</button>
				{/snippet}
				{#snippet additConnectedServerViewActions(connectedServer)}
					{@render connectedActions(connectedServer)}
				{/snippet}
				{#snippet additConnectedServerCardActions(connectedServer)}
					{@const configured = connectedServer?.server?.configured}
					<button
						class="menu-button"
						onclick={async () => {
							connectToServer = connectedServer;
							connectDialog?.open();
						}}
						disabled={!configured}
					>
						Connect To Server
					</button>
					{@render connectedActions(connectedServer)}
				{/snippet}
			</MyMcpServers>
		</div>
	</div>
</Layout>

{#snippet connectedActions(connectedServer: typeof connectToServer)}
	{@const configured = connectedServer?.server?.configured}
	<button
		class="menu-button justify-between"
		disabled={!configured}
		onclick={() => {
			if (!connectedServer) return;
			handleSetupChat(connectedServer);
		}}
	>
		Chat <ExternalLink class="size-4 -translate-y-[1px]" />
	</button>
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		{#if connectToServer}
			{@const name = connectToServer.server?.manifest.name ?? ''}
			{@const icon = connectToServer.server?.manifest.icon ?? ''}

			<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
				{#if icon}
					<img src={icon} alt={name} class="size-8" />
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			{name}
		{/if}
	{/snippet}

	{#if connectToServer}
		{@const url = connectToServer.connectURL}
		{@const name = connectToServer.server?.manifest.name}
		<div class="flex items-center gap-4">
			<div class="mb-4 flex grow flex-col gap-1">
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
			<div class="w-32">
				<button
					class="button-primary flex h-fit w-full grow items-center justify-center gap-2 text-sm"
					onclick={() => handleSetupChat(connectToServer)}
				>
					Chat <ExternalLink class="size-4" />
				</button>
			</div>
		</div>

		{#if url && name}
			<HowToConnect servers={[{ url, name }]} />
		{/if}
	{/if}
</ResponsiveDialog>

<ResponsiveDialog bind:this={showAllServersConfigDialog}>
	{#snippet titleContent()}
		Connect to Your Servers
	{/snippet}

	<p class="text-md mb-8">
		Select your preferred AI tooling below and copy & paste the configuration to get set up with all
		your connected servers.
	</p>

	<HowToConnect
		servers={userConfiguredServers.map((server) => ({
			url: server.connectURL ?? '',
			name: server.manifest.name ?? ''
		}))}
	/>
</ResponsiveDialog>

<PageLoading show={chatLoading} text="Loading chat..." />

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
