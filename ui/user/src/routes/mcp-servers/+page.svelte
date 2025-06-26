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
	import { type MCPServerInfo } from '$lib/services/chat/mcp';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services/index.js';
	import { ChevronLeft, ChevronRight, LoaderCircle, Server, Unplug } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import McpServerInfo from '$lib/components/mcp/McpServerInfo.svelte';

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);

	let deletingInstance = $state<MCPServerInstance>();
	let connectToEntry = $state<{
		entry: MCPCatalogEntry;
		envs: MCPServerInfo['env'];
		headers: MCPServerInfo['headers'];
		url: MCPServerInfo['url'];
		connectURL?: string;
		launching: boolean;
	}>();
	let connectToServer = $state<{
		server: MCPCatalogServer;
		userConfiguredServer?: MCPCatalogServer;
		connectURL?: string;
	}>();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let serverInfoDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editUserConfiguredServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let userConfiguredServerToEdit = $state<{
		id: string;
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
		icon?: string;
		name?: string;
	}>();

	let search = $state('');
	let serverInstancesMap = $derived(
		new Map(
			userServerInstances.map((instance) => [
				(instance.mcpServerID ?? instance.mcpCatalogID) as string,
				instance
			])
		)
	);
	let userConfiguredServersMap = $derived(
		new Map(userConfiguredServers.map((server) => [server.catalogEntryID, server]))
	);
	let filteredEntriesData = $derived(
		entries.filter((item) => {
			const userConfiguredServer = userConfiguredServersMap.get(item.id);
			if (userConfiguredServer && serverInstancesMap.has(userConfiguredServer.id)) {
				return false;
			}

			if (search) {
				const nameToUse = item.commandManifest?.name ?? item.urlManifest?.name;
				return nameToUse?.toLowerCase().includes(search.toLowerCase());
			}

			return true;
		})
	);
	let filteredServers = $derived(
		servers.filter((item) => {
			if (serverInstancesMap.has(item.id)) {
				return false;
			}

			if (search) {
				return item.manifest.name?.toLowerCase().includes(search.toLowerCase());
			}

			return true;
		})
	);
	let filteredData = $derived([...filteredServers, ...filteredEntriesData]);
	let connectedServers = $derived(
		userServerInstances.map((instance) => {
			const userConfiguredServer = userConfiguredServers.find((s) => s.id === instance.mcpServerID);
			return {
				instance,
				userConfiguredServer,
				parent: userConfiguredServer?.catalogEntryID
					? entries.find((e) => e.id === userConfiguredServer.catalogEntryID)
					: servers.find((s) => s.id === instance.mcpServerID)
			};
		})
	);

	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

	async function loadData() {
		loading = true;
		try {
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
		} catch (error) {
			console.error('Failed to load data:', error);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadData();
	});

	function parseCategories(item: (typeof filteredData)[0]) {
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

	function convertEnvHeadersToRecord(
		envs: MCPServerInfo['env'],
		headers: MCPServerInfo['headers']
	) {
		const secretValues: Record<string, string> = {};
		for (const env of envs ?? []) {
			if (env.value) {
				secretValues[env.key] = env.value;
			}
		}

		for (const header of headers ?? []) {
			if (header.value) {
				secretValues[header.key] = header.value;
			}
		}
		return secretValues;
	}

	async function handleMcpServer(server: MCPCatalogServer, instance?: MCPServerInstance) {
		connectToServer = {
			server,
			userConfiguredServer: userConfiguredServersMap.get(server.id),
			connectURL: instance?.connectURL
		};

		if (connectToServer.connectURL) {
			connectDialog?.open();
		} else {
			serverInfoDialog?.open();
		}
	}

	async function handleMcpEntry(entry: MCPCatalogEntry, instance?: MCPServerInstance) {
		const envs = (
			(entry.commandManifest ? entry.commandManifest.env : entry.urlManifest?.env) ?? []
		).map((env) => ({ ...env, value: '' }));

		const headers = (
			(entry.commandManifest ? entry.commandManifest.headers : entry.urlManifest?.headers) ?? []
		).map((header) => ({ ...header, value: '' }));

		const url = entry.urlManifest?.fixedURL ?? '';

		connectToEntry = { entry, envs, headers, url, launching: false };

		if (instance) {
			connectToEntry.connectURL = instance.connectURL;
			connectDialog?.open();
		} else {
			serverInfoDialog?.open();
		}
	}

	async function handleLaunch() {
		if (connectToEntry) {
			connectToEntry.launching = true;
			const manifest = connectToEntry.entry.commandManifest ?? connectToEntry.entry.urlManifest;
			if (!manifest) {
				console.error('No server manifest found');
				return;
			}

			const url = connectToEntry.url;

			const response = await ChatService.createSingleOrRemoteMcpServer({
				catalogEntryID: connectToEntry.entry.id,
				...(connectToEntry.entry.urlManifest ? { manifest: { url } } : {})
			});
			const instance = await ChatService.createMcpServerInstance(response.id);

			const secretValues = convertEnvHeadersToRecord(connectToEntry.envs, connectToEntry.headers);
			await ChatService.configureSingleOrRemoteMcpServer(response.id, secretValues);
			connectToEntry.connectURL = instance.connectURL;
			connectToEntry.launching = false;
			configDialog?.close();
			connectDialog?.open();
		} else if (connectToServer) {
			const instance = await ChatService.createMcpServerInstance(connectToServer.server.id);
			connectToServer.connectURL = instance.connectURL;
			connectDialog?.open();
		}

		await loadData();
	}

	function handleSelectItem(item: (typeof filteredData)[0]) {
		connectToEntry = undefined;
		connectToServer = undefined;

		if (item.type === 'mcpserver') {
			handleMcpServer(item as MCPCatalogServer, serverInstancesMap.get(item.id));
		} else {
			const userConfiguredServer = userConfiguredServersMap.get(item.id);
			handleMcpEntry(
				item as MCPCatalogEntry,
				userConfiguredServer ? serverInstancesMap.get(userConfiguredServer.id) : undefined
			);
		}
	}

	function hasEditableConfiguration(item: MCPCatalogEntry) {
		const userConfiguredServer = userConfiguredServersMap.get(item.id);
		if (!userConfiguredServer) {
			return false;
		}

		const hasEnvs =
			userConfiguredServer.manifest.env && userConfiguredServer.manifest.env.length > 0;
		const hasHeaders =
			userConfiguredServer.manifest.headers && userConfiguredServer.manifest.headers.length > 0;
		return hasEnvs || hasHeaders;
	}
</script>

<Layout showUserLinks>
	<div class="flex flex-col gap-8 pt-4" in:fade>
		<h1 class="text-2xl font-semibold">MCP Servers</h1>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			{#if connectedServers.length > 0}
				<div class="flex flex-col gap-4">
					<h2 class="text-lg font-semibold">Connected MCP Servers</h2>
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
						{#each connectedServers as connectedServer}
							{#if connectedServer.parent}
								{@render mcpServerCard(connectedServer.parent, connectedServer.instance)}
							{/if}
						{/each}
					</div>
				</div>
			{/if}
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
					{#each paginatedData as item}
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

{#snippet mcpServerCard(item: (typeof filteredData)[0], instance?: MCPServerInstance)}
	{@const icon =
		'manifest' in item
			? item.manifest.icon
			: (item.commandManifest?.icon ?? item.urlManifest?.icon)}
	{@const name =
		'manifest' in item
			? item.manifest.name
			: (item.commandManifest?.name ?? item.urlManifest?.name)}
	{@const categories = parseCategories(item)}
	<div class="relative flex flex-col">
		<button
			class="dark:bg-surface1 dark:border-surface3 flex h-full w-full flex-col rounded-sm border border-transparent bg-white px-2 py-4 text-left shadow-sm"
			onclick={() => {
				if (!instance) {
					handleSelectItem(item);
				} else {
					connectToEntry = undefined;
					connectToServer = {
						server: 'manifest' in item ? item : userConfiguredServersMap.get(item.id)!,
						userConfiguredServer: 'manifest' in item ? item : userConfiguredServersMap.get(item.id)
					};
					serverInfoDialog?.open();
				}
			}}
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
		</button>
		<div
			class="absolute -top-2 right-0 flex h-full translate-y-2 flex-col justify-between gap-4 p-2"
		>
			{#if instance}
				<DotDotDot
					class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
				>
					<div class="default-dialog flex min-w-max flex-col p-2">
						<button
							class="menu-button"
							onclick={() => {
								handleSelectItem(item);
							}}
						>
							Get Connection URL
						</button>
						{#if !('manifest' in item) && hasEditableConfiguration(item)}
							<button
								class="menu-button"
								onclick={async () => {
									const userConfiguredServer = userConfiguredServersMap.get(item.id);
									if (!userConfiguredServer) {
										console.error('No user configured server for this entry found');
										return;
									}

									let values: Record<string, string>;
									try {
										values = await ChatService.revealSingleOrRemoteMcpServer(
											userConfiguredServer.id
										);
									} catch (error) {
										if (error instanceof Error && !error.message.includes('404')) {
											console.error(
												'Failed to reveal user server values due to unexpected error',
												error
											);
										}
										values = {};
									}

									userConfiguredServerToEdit = {
										id: userConfiguredServer.id,
										envs: userConfiguredServer.manifest.env?.map((env) => ({
											...env,
											value: values[env.key] ?? ''
										})),
										headers: userConfiguredServer.manifest.headers?.map((header) => ({
											...header,
											value: values[header.key] ?? ''
										})),
										url: userConfiguredServer.manifest.url,
										icon: userConfiguredServer.manifest.icon,
										name: userConfiguredServer.manifest.name
									};
									editUserConfiguredServerDialog?.open();
								}}
							>
								Edit Configuration
							</button>
						{/if}
						<button
							class="menu-button text-red-500"
							onclick={async () => {
								deletingInstance = instance;
							}}
						>
							Disconnect
						</button>
					</div>
				</DotDotDot>
			{:else}
				<button
					class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
					use:tooltip={'Connect to server'}
					onclick={() => {
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
		<McpServerInfo entry={connectToEntry.entry as MCPCatalogEntry} />
	{/if}

	{#if connectToServer}
		<McpServerInfo entry={connectToServer.userConfiguredServer ?? connectToServer.server} />
	{/if}
	<div class="mt-4 flex justify-end">
		{#if connectToEntry || (connectToServer && !connectToServer.userConfiguredServer)}
			<button
				class="button-primary"
				onclick={() => {
					serverInfoDialog?.close();
					if (connectToEntry) {
						const hasUrlToFill =
							connectToEntry.entry.urlManifest && connectToEntry.entry.urlManifest.hostname;
						const hasEnvsToFill = connectToEntry.envs && connectToEntry.envs.length > 0;
						const hasHeadersToFill = connectToEntry.headers && connectToEntry.headers.length > 0;
						if (hasUrlToFill || hasEnvsToFill || hasHeadersToFill) {
							configDialog?.open();
						} else {
							handleLaunch();
						}
					} else if (connectToServer) {
						handleLaunch();
					}
				}}
			>
				Connect
			</button>
		{/if}
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
			{@render configureForm(connectToEntry, connectToEntry.entry)}
			<div class="flex justify-end">
				<button class="button-primary" onclick={handleLaunch}>Launch</button>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={editUserConfiguredServerDialog}
	onClose={() => (userConfiguredServerToEdit = undefined)}
>
	{#snippet titleContent()}
		{#if userConfiguredServerToEdit?.icon}
			<img
				src={userConfiguredServerToEdit.icon}
				alt={userConfiguredServerToEdit.name}
				class="size-6"
			/>
		{:else}
			<Server class="size-6" />
		{/if}
		{userConfiguredServerToEdit?.name}
	{/snippet}
	{#if userConfiguredServerToEdit}
		{@render configureForm(userConfiguredServerToEdit)}
		<div class="flex justify-end">
			<button
				class="button-primary"
				onclick={async () => {
					if (userConfiguredServerToEdit) {
						const secretValues = convertEnvHeadersToRecord(
							userConfiguredServerToEdit.envs,
							userConfiguredServerToEdit.headers
						);
						await ChatService.configureSingleOrRemoteMcpServer(
							userConfiguredServerToEdit.id,
							secretValues
						);
						editUserConfiguredServerDialog?.close();
						await loadData();
					}
				}}>Update</button
			>
		</div>
	{/if}
</ResponsiveDialog>

{#snippet configureForm(
	fields: {
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
	},
	entry?: MCPCatalogEntry
)}
	<div class="my-4 flex flex-col gap-4">
		{#if fields.envs && fields.envs.length > 0}
			{#each fields.envs as env, i}
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
						<SensitiveInput name={env.name} bind:value={fields.envs[i].value} />
					{:else}
						<input
							type="text"
							id={env.key}
							bind:value={fields.envs[i].value}
							class="text-input-filled"
						/>
					{/if}
				</div>
			{/each}
		{/if}
		{#if fields.headers && fields.headers.length > 0}
			{#each fields.headers as header, i}
				<div class="flex flex-col gap-1">
					<span class="flex items-center gap-2">
						<label for={header.key}>
							{header.name}
							{#if !header.required}
								<span class="text-gray-400 dark:text-gray-600">(optional)</span>
							{/if}
						</label>
						<InfoTooltip text={header.description} />
					</span>
					{#if header.sensitive}
						<SensitiveInput name={header.name} bind:value={fields.headers[i].value} />
					{:else}
						<input
							type="text"
							id={header.key}
							bind:value={fields.headers[i].value}
							class="text-input-filled"
						/>
					{/if}
				</div>
			{/each}
		{/if}
		{#if entry?.urlManifest || fields.url}
			<label for="url-manifest-url"> URL </label>
			<input type="text" id="url-manifest-url" bind:value={fields.url} class="text-input-filled" />
			{#if entry?.urlManifest?.hostname}
				<span class="font-light text-gray-400 dark:text-gray-600">
					The URL must contain the hostname: <b class="font-semibold">
						{entry.urlManifest.hostname}
					</b>
				</span>
			{/if}
		{/if}
	</div>
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry?.connectURL}
		{@render connectUrlButton(connectToEntry.connectURL)}
	{:else if connectToServer?.connectURL}
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
		{#if connectToServer.server.manifest.icon}
			<img
				src={connectToServer.server.manifest.icon}
				alt={connectToServer.server.manifest.name}
				class="size-6"
			/>
		{:else}
			<Server class="size-6" />
		{/if}
		{connectToServer.server.manifest.name}
	{/if}
{/snippet}

<Confirm
	msg={'Are you sure you want to delete this server?'}
	show={Boolean(deletingInstance)}
	onsuccess={async () => {
		if (deletingInstance) {
			if (deletingInstance.mcpCatalogID) {
				// find & delete user server
				const matchingUserServer = userConfiguredServers.find(
					(server) => server.id === deletingInstance?.mcpCatalogID
				);
				if (matchingUserServer) {
					await ChatService.deleteSingleOrRemoteMcpServer(matchingUserServer.id);
				}
			}
			await ChatService.deleteMcpServerInstance(deletingInstance.id);
			// TODO: does loadData need to happen or can it one or two calls to reload
			await loadData();
			deletingInstance = undefined;
		}
	}}
	oncancel={() => (deletingInstance = undefined)}
/>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
