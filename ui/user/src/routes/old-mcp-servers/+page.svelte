<script lang="ts">
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { parseCategories, requiresUserUpdate } from '$lib/services/chat/mcp';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services/index.js';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { afterNavigate } from '$app/navigation';
	import MyMcpServers, { type ConnectedServer } from '$lib/components/mcp/MyMcpServers.svelte';
	import { responsive } from '$lib/stores';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);

	let connectToServer = $state<{
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
		connectURL?: string;
		parent?: MCPCatalogEntry;
	}>();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let showAllServersConfigDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let myMcpServers = $state<ReturnType<typeof MyMcpServers>>();
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

	let categories = $derived(
		[
			...new Set([
				...convertedEntries.flatMap((item) => item.categories),
				...convertedServers.flatMap((item) => item.categories)
			])
		].sort((a, b) => a.localeCompare(b))
	);

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
			userConfiguredServers = userConfiguredServers.filter(
				(server) => !server.deleted && !server.powerUserWorkspaceID
			);
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
</script>

<Layout showUserLinks navLinks={[]} hideSidebar classes={{ container: 'pb-0' }}>
	<div class="flex h-full w-full">
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
								myMcpServers?.reset();
							}}
						>
							{category}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex w-full flex-col gap-8 px-2 pt-4" in:fade>
			<h1 class="text-2xl font-semibold">
				{selectedCategory ? selectedCategory : 'My Connectors'}
			</h1>
			<MyMcpServers
				bind:this={myMcpServers}
				{userServerInstances}
				userConfiguredServers={convertedUserConfiguredServers}
				servers={convertedServers}
				entries={convertedEntries}
				{loading}
				onConnectServer={(connectedServer) => {
					loadData(true);
					connectToServer = connectedServer;
					connectToServerDialog?.open();
				}}
				onSelectConnectedServer={(connectedServer) => {
					connectToServer = connectedServer;
					connectToServerDialog?.open();
				}}
				onDisconnect={() => {
					loadData(true);
				}}
				connectSelectText="Connect"
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
					{@const requiresUpdate = requiresUserUpdate(connectedServer)}
					{@render connectedActions(connectedServer)}
					<button
						class="menu-button"
						onclick={async () => {
							connectToServer = connectedServer;
							connectToServerDialog?.open();
						}}
						disabled={requiresUpdate}
					>
						Connect
					</button>
				{/snippet}
			</MyMcpServers>
		</div>
	</div>
</Layout>

{#snippet connectedActions(connectedServer: ConnectedServer)}
	{@const requiresUpdate = requiresUserUpdate(connectedServer)}
	<button
		class="menu-button justify-between"
		disabled={requiresUpdate}
		onclick={() => {
			if (!connectedServer?.server) return;
			connectToServerDialog?.handleSetupChat(connectedServer.server, connectedServer.instance);
		}}
	>
		Chat
	</button>
{/snippet}

<ConnectToServer
	bind:this={connectToServerDialog}
	server={connectToServer?.server}
	instance={connectToServer?.instance}
/>

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
			name: (server.alias || server.manifest.name) ?? ''
		}))}
	/>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
