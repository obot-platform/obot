<script lang="ts">
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { createProjectMcp, parseCategories, requiresUserUpdate } from '$lib/services/chat/mcp';
	import {
		ChatService,
		EditorService,
		Group,
		type AccessControlRule,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services/index.js';
	import { ExternalLink, Plus, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import PageLoading from '$lib/components/PageLoading.svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import MyMcpServers, { type ConnectedServer } from '$lib/components/mcp/MyMcpServers.svelte';
	import { MCP_PUBLISHER_ALL_OPTION, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte';
	import { profile } from '$lib/stores/index.js';
	import { workspaceStore } from '$lib/stores/workspace.svelte.js';
	import { browser } from '$app/environment';

	initMcpServerAndEntries();

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);

	let chatLoading = $state(false);
	let chatLoadingProgress = $state(0);
	let chatLaunchError = $state<string>();

	let showCreateRegistry = $state(false);
	let selectedItemTitle = $state<string>('');

	let connectToServer = $state<{
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
		connectURL?: string;
		parent?: MCPCatalogEntry;
	}>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let showAllServersConfigDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let myMcpServers = $state<ReturnType<typeof MyMcpServers>>();

	let hasAccessToCreateRegistry = $derived(profile.current?.groups?.includes(Group.POWERUSER_PLUS));
	let workspace = $derived($workspaceStore);

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

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const initialSelectedId = to.url.searchParams.get('id');
			const showCreate = to.url.searchParams.get('new') === 'true';
			if (showCreate) {
				showCreateRegistry = true;
				selectedItemTitle = '';
				myMcpServers?.reset();
			} else if (!initialSelectedId && (selectedItemTitle || showCreateRegistry)) {
				showCreateRegistry = false;
				selectedItemTitle = '';
				myMcpServers?.reset();
			}
		}
	});

	async function handleSetupChat(connectedServer: typeof connectToServer) {
		if (!connectedServer || !connectedServer.server) return;
		connectDialog?.close();
		chatLaunchError = undefined;
		chatLoading = true;
		chatLoadingProgress = 0;

		let timeout1 = setTimeout(() => {
			chatLoadingProgress = 10;
		}, 1000);
		let timeout2 = setTimeout(() => {
			chatLoadingProgress = 50;
		}, 5000);
		let timeout3 = setTimeout(() => {
			chatLoadingProgress = 80;
		}, 10000);

		const projects = await ChatService.listProjects();
		const name = [
			connectedServer.server?.alias || connectedServer.server?.manifest.name || '',
			connectedServer.server.id
		].join(' - ');
		const match = projects.items.find((project) => project.name === name);

		let project = match;
		if (!match) {
			// if no project match, create a new one w/ mcp server connected to it
			project = await EditorService.createObot({
				name: name
			});
		}

		try {
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
		} catch (err) {
			chatLaunchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}

		chatLoadingProgress = 100;
		setTimeout(() => {
			chatLoading = false;
			goto(`/o/${project?.id}`);
		}, 1000);
	}

	function handleSelectCreateRegistry() {
		if (workspace.id) {
			fetchMcpServerAndEntries(workspace.id);
		}
		goto('/mcp-registry?new=true');
	}

	async function navigateToCreated(rule: AccessControlRule) {
		workspaceStore.fetchData(true);
		showCreateRegistry = false;
		goto(`/mcp-registry/r/${rule.id}`, { replaceState: false });
	}

	function getCurrentTitle() {
		if (selectedItemTitle) {
			return selectedItemTitle;
		}

		if (showCreateRegistry) {
			return 'Create New Registry';
		}

		return workspace.rules.length > 0 ? 'Shared with Me' : 'MCP Registry';
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout
	title={getCurrentTitle()}
	showBackButton={showCreateRegistry || Boolean(selectedItemTitle)}
	onBackButtonClick={() => {
		goto('/mcp-registry');
	}}
>
	<div class="flex h-full w-full">
		{#if showCreateRegistry}
			{@render createRegistryScreen()}
		{:else}
			<div
				class="w-full pt-4"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
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
						connectDialog?.open();
					}}
					onSelectConnectedServer={(connectedServer) => {
						connectToServer = connectedServer;
						connectDialog?.open();
					}}
					onDisconnect={() => {
						loadData(true);
					}}
					connectSelectText="Connect"
					onUpdateConfigure={() => {
						loadData(true);
					}}
					onSelectCard={(item) => {
						selectedItemTitle =
							'server' in item
								? item.server?.alias || item.server?.manifest.name || ''
								: 'manifest' in item
									? item.manifest?.name || ''
									: '';
						const id = 'server' in item ? item.server?.id : 'manifest' in item ? item.id : '';
						goto(`/mcp-registry?id=${id}`);
					}}
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
								connectDialog?.open();
							}}
							disabled={requiresUpdate}
						>
							Connect
						</button>
					{/snippet}
				</MyMcpServers>
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if hasAccessToCreateRegistry && !selectedItemTitle && !showCreateRegistry}
			<button
				class="button-primary flex h-fit items-center gap-2 text-sm"
				onclick={handleSelectCreateRegistry}
			>
				<Plus class="size-4" />
				New Registry
			</button>
		{/if}
	{/snippet}
</Layout>

{#snippet createRegistryScreen()}
	<div
		class="h-full w-full pt-4"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<AccessControlRuleForm
			onCreate={navigateToCreated}
			entity="workspace"
			id={workspace.id}
			mcpEntriesContextFn={getPoweruserWorkspace}
			all={MCP_PUBLISHER_ALL_OPTION}
		/>
	</div>
{/snippet}

{#snippet connectedActions(connectedServer: ConnectedServer)}
	{@const requiresUpdate = requiresUserUpdate(connectedServer)}
	<button
		class="menu-button justify-between"
		disabled={requiresUpdate}
		onclick={() => {
			if (!connectedServer) return;
			handleSetupChat(connectedServer);
		}}
	>
		Chat
	</button>
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		{#if connectToServer}
			{@const nameToShow =
				connectToServer.server?.alias || connectToServer.server?.manifest.name || ''}
			{@const icon = connectToServer.server?.manifest.icon ?? ''}

			<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
				{#if icon}
					<img src={icon} alt={nameToShow} class="size-8" />
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			{nameToShow}
		{/if}
	{/snippet}

	{#if connectToServer}
		{@const url = connectToServer.connectURL}
		{@const nameToShow =
			connectToServer.server?.alias || connectToServer.server?.manifest.name || ''}
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

		{#if url}
			<HowToConnect servers={[{ url, name: nameToShow }]} />
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
			name: (server.alias || server.manifest.name) ?? ''
		}))}
	/>
</ResponsiveDialog>

<PageLoading
	show={chatLoading}
	isProgressBar
	progress={chatLoadingProgress}
	text="Loading chat..."
	error={chatLaunchError}
	longLoadMessage="Connecting MCP Server to chat..."
	longLoadDuration={10000}
	onClose={() => {
		chatLoading = false;
	}}
/>

<svelte:head>
	<title>Obot | MCP Registry</title>
</svelte:head>
