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
		EditorService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services/index.js';
	import {
		ChevronLeft,
		ChevronRight,
		ExternalLink,
		LoaderCircle,
		Server,
		Unplug
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import McpServerInfo from '$lib/components/mcp/McpServerInfo.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import { twMerge } from 'tailwind-merge';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import PageLoading from '$lib/components/PageLoading.svelte';

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(true);
	let chatLoading = $state(false);

	let deletingInstance = $state<MCPServerInstance>();
	let deletingServer = $state<MCPCatalogServer>();
	let connectToEntry = $state<{
		entry: MCPCatalogEntry;
		envs: MCPServerInfo['env'];
		headers: MCPServerInfo['headers'];
		url: MCPServerInfo['url'];
		connectURL?: string;
		launching?: boolean;
	}>();
	let connectToServer = $state<{
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
		connectURL?: string;
		parent?: MCPCatalogEntry;
	}>();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
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
	let showAllServersConfigDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let showServerInfo = $state(false);

	let search = $state('');
	let highlightedFields = $state<Set<string>>(new Set());
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
			if (item.deleted) {
				return false;
			}

			const userConfiguredServer = userConfiguredServersMap.get(item.id);
			if (userConfiguredServer) {
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
			if (item.deleted) {
				return false;
			}

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
	let connectedServers = $derived([
		...userConfiguredServers
			.filter((server) => server.connectURL && !server.deleted)
			.map((server) => ({
				connectURL: server.connectURL ?? '',
				server,
				instance: undefined,
				parent: server.catalogEntryID
					? (entries.find((e) => e.id === server.catalogEntryID) ?? undefined)
					: undefined
			})),
		...userServerInstances.map((instance) => ({
			connectURL: instance.connectURL ?? '',
			instance,
			server: servers.find((s) => s.id === instance.mcpServerID) ?? undefined,
			parent: undefined
		}))
	]);

	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

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

	function parseCategories(item?: (typeof filteredData)[0] | null) {
		if (!item) return [];
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

			const secretValues = convertEnvHeadersToRecord(connectToEntry.envs, connectToEntry.headers);
			const configuredResponse = await ChatService.configureSingleOrRemoteMcpServer(
				response.id,
				secretValues
			);

			connectToServer = {
				server: configuredResponse,
				connectURL: configuredResponse.connectURL,
				instance: undefined,
				parent: connectToEntry?.entry
			};
			connectToEntry = undefined;
			clearHighlights();
			configDialog?.close();
			connectDialog?.open();
		} else if (connectToServer?.server) {
			const instance = await ChatService.createMcpServerInstance(connectToServer.server.id);
			connectToServer.connectURL = instance.connectURL;
			connectDialog?.open();
		}

		await loadData(true);
	}

	function handleSelectItem(item: (typeof filteredData)[0]) {
		connectToServer = undefined;
		connectToEntry = undefined;

		if ('manifest' in item) {
			connectToServer = {
				server: item as MCPCatalogServer
			};
		} else {
			const manifest = item.commandManifest ?? item.urlManifest;
			const envs = (manifest?.env ?? []).map((env) => ({ ...env, value: '' }));
			const headers = (manifest?.headers ?? []).map((header) => ({ ...header, value: '' }));
			const url = manifest?.fixedURL ?? '';
			connectToEntry = { entry: item, envs, headers, url, launching: false };
		}
		showServerInfo = true;
	}

	function hasEditableConfiguration(item: MCPCatalogEntry) {
		const manifest = item.commandManifest ?? item.urlManifest;
		const hasUrlToFill = !manifest?.fixedURL && manifest?.hostname;
		const hasEnvsToFill = manifest?.env && manifest.env.length > 0;
		const hasHeadersToFill = manifest?.headers && manifest.headers.length > 0;

		return hasUrlToFill || hasEnvsToFill || hasHeadersToFill;
	}

	function getCurrentName() {
		if (connectToEntry) {
			return (
				connectToEntry.entry.commandManifest?.name ?? connectToEntry.entry.urlManifest?.name ?? ''
			);
		}
		if (connectToServer) {
			return connectToServer.server?.manifest.name ?? '';
		}
		return '';
	}

	function getCurrentIcon() {
		if (connectToEntry) {
			return (
				connectToEntry.entry.commandManifest?.icon ?? connectToEntry.entry.urlManifest?.icon ?? ''
			);
		}
		if (connectToServer) {
			return connectToServer.server?.manifest.icon ?? '';
		}
		return '';
	}

	async function handleSetupChat(connectedServer: typeof connectToServer) {
		if (!connectedServer || !connectedServer.server) return;
		chatLoading = true;

		const projects = await ChatService.listProjects();
		const match = projects.items.find(
			(project) => project.name === connectedServer.server?.manifest.name
		);

		let project = match;
		if (!match) {
			// if no project match, create a new one w/ mcp server connected to it
			project = await EditorService.createObot({
				name: connectedServer.server?.manifest.name ?? ''
			});
		}

		if (
			project &&
			!(await ChatService.listProjectMCPs(project.assistantID, project.id)).find(
				(mcp) => mcp.manifest.name === connectedServer.server?.manifest.name
			)
		) {
			const mcpServerInfo = {
				manifest: {
					name: connectedServer.server.manifest.name,
					icon: connectedServer.server.manifest.icon,
					description: connectedServer.server.manifest.description,
					metadata: connectedServer.server.manifest.metadata,
					url: connectedServer.connectURL
				}
			} as MCPServerInfo;

			await createProjectMcp(mcpServerInfo, project);
		}

		window.open(`/o/${project?.id}`, '_blank');
		chatLoading = false;
	}

	function highlightMissingRequiredFields() {
		const fieldsToHighlight = new Set<string>();

		if (connectToEntry) {
			// Check required envs
			connectToEntry.envs?.forEach((env) => {
				if (env.required && !env.value) {
					fieldsToHighlight.add(env.key);
				}
			});

			// Check required headers
			connectToEntry.headers?.forEach((header) => {
				if (header.required && !header.value) {
					fieldsToHighlight.add(header.key);
				}
			});

			// Check URL for URL manifest entries
			if (connectToEntry.entry.urlManifest && !connectToEntry.url) {
				fieldsToHighlight.add('url-manifest-url');
			}
		}

		highlightedFields = fieldsToHighlight;
	}

	function clearHighlights() {
		highlightedFields = new Set();
	}

	// Clear highlights when values change for sensitive inputs
	$effect(() => {
		if (connectToEntry) {
			connectToEntry.envs?.forEach((env) => {
				if (env.value && highlightedFields.has(env.key)) {
					highlightedFields.delete(env.key);
					highlightedFields = new Set(highlightedFields);
				}
			});
			connectToEntry.headers?.forEach((header) => {
				if (header.value && highlightedFields.has(header.key)) {
					highlightedFields.delete(header.key);
					highlightedFields = new Set(highlightedFields);
				}
			});
			if (connectToEntry.url && highlightedFields.has('url-manifest-url')) {
				highlightedFields.delete('url-manifest-url');
				highlightedFields = new Set(highlightedFields);
			}
		}

		if (userConfiguredServerToEdit) {
			userConfiguredServerToEdit.envs?.forEach((env) => {
				if (env.value && highlightedFields.has(env.key)) {
					highlightedFields.delete(env.key);
					highlightedFields = new Set(highlightedFields);
				}
			});
			userConfiguredServerToEdit.headers?.forEach((header) => {
				if (header.value && highlightedFields.has(header.key)) {
					highlightedFields.delete(header.key);
					highlightedFields = new Set(highlightedFields);
				}
			});
			if (userConfiguredServerToEdit.url && highlightedFields.has('url-manifest-url')) {
				highlightedFields.delete('url-manifest-url');
				highlightedFields = new Set(highlightedFields);
			}
		}
	});

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout showUserLinks>
	<div class="flex flex-col gap-8 pt-4" in:fade>
		{#if showServerInfo}
			{@render serverContent()}
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
		<h1 class="text-2xl font-semibold">MCP Servers</h1>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			{#if connectedServers.length > 0}
				<div class="flex flex-col gap-4">
					<div class="flex items-center gap-4">
						<h2 class="text-lg font-semibold">Connected MCP Servers</h2>
						<button class="button text-xs" onclick={() => showAllServersConfigDialog?.open()}>
							Generate Configuration
						</button>
					</div>
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
						{#each connectedServers as connectedServer, i (i)}
							{@render connectedMcpServerCard(connectedServer)}
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
					{#each paginatedData as item (item.id)}
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
{/snippet}

{#snippet connectedMcpServerCard(connectedServer: (typeof connectedServers)[0])}
	{@const icon = connectedServer.server?.manifest.icon}
	{@const name = connectedServer.server?.manifest.name}
	{@const description = connectedServer.server?.manifest.description}
	{@const categories = parseCategories(connectedServer.server ?? connectedServer.parent)}
	<div class="relative flex flex-col">
		<button
			class="dark:bg-surface1 dark:border-surface3 flex h-full w-full flex-col rounded-sm border border-transparent bg-white p-3 text-left shadow-sm"
			onclick={() => {
				connectToServer = undefined;
				connectToEntry = undefined;

				if (connectedServer.parent) {
					connectToServer = {
						server: connectedServer.server,
						connectURL: connectedServer.connectURL,
						parent: connectedServer.parent
					};
				} else {
					connectToServer = {
						server: connectedServer.server,
						instance: connectedServer.instance,
						connectURL: connectedServer.connectURL
					};
				}
				showServerInfo = true;
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
				<div class="flex max-w-[calc(100%-2rem)] flex-col">
					<p class="text-sm font-semibold">{name}</p>
					<span
						class={twMerge(
							'text-xs leading-4.5 font-light text-gray-400 dark:text-gray-600',
							categories.length > 0 ? 'line-clamp-2' : 'line-clamp-3'
						)}
					>
						{stripMarkdownToText(description ?? '')}
					</span>
				</div>
			</div>
			<div class="flex w-full flex-wrap gap-1 pt-2">
				{#each categories as category (category)}
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
			<DotDotDot
				class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
			>
				<div class="default-dialog flex min-w-max flex-col p-2">
					<button
						class="menu-button"
						onclick={() => {
							connectToEntry = undefined;
							connectToServer = {
								...connectedServer
							};
							connectDialog?.open();
						}}
					>
						Get Connection URL
					</button>
					{#if connectedServer.parent && hasEditableConfiguration(connectedServer.parent)}
						<button
							class="menu-button"
							onclick={async () => {
								let values: Record<string, string>;
								try {
									values = await ChatService.revealSingleOrRemoteMcpServer(
										connectedServer.server.id
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
									id: connectedServer.server.id,
									envs: connectedServer.server.manifest.env?.map((env) => ({
										...env,
										value: values[env.key] ?? ''
									})),
									headers: connectedServer.server.manifest.headers?.map((header) => ({
										...header,
										value: values[header.key] ?? ''
									})),
									url: connectedServer.server.manifest.url,
									icon: connectedServer.server.manifest.icon,
									name: connectedServer.server.manifest.name
								};
								editUserConfiguredServerDialog?.open();
							}}
						>
							Edit Configuration
						</button>
					{/if}
					<button
						class="menu-button justify-between"
						onclick={() => handleSetupChat(connectedServer)}
					>
						Chat <ExternalLink class="size-4 -translate-y-[1px]" />
					</button>
					<button
						class="menu-button text-red-500"
						onclick={async () => {
							if (connectedServer.instance) {
								deletingInstance = connectedServer.instance;
							} else if (connectedServer.parent) {
								deletingServer = connectedServer.server;
							}
						}}
					>
						Disconnect
					</button>
				</div>
			</DotDotDot>
		</div>
	</div>
{/snippet}

{#snippet mcpServerCard(item: (typeof filteredData)[0])}
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
			class="dark:bg-surface1 dark:border-surface3 flex h-full w-full flex-col rounded-sm border border-transparent bg-white p-3 text-left shadow-sm"
			onclick={() => {
				handleSelectItem(item);
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
				<div class="flex max-w-[calc(100%-2rem)] flex-col">
					<p class="text-sm font-semibold">{name}</p>
					<span
						class={twMerge(
							'text-xs leading-4.5 font-light text-gray-400 dark:text-gray-600',
							categories.length > 0 ? 'line-clamp-2' : 'line-clamp-3'
						)}
					>
						{#if 'manifest' in item}
							{stripMarkdownToText(item.manifest.description ?? '')}
						{:else}
							{stripMarkdownToText(
								item.commandManifest?.description ?? item.urlManifest?.description ?? ''
							)}
						{/if}
					</span>
				</div>
			</div>
			<div class="flex w-full flex-wrap gap-1 pt-2">
				{#each categories as category (category)}
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
			<button
				class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
				use:tooltip={'Connect to server'}
				onclick={() => {
					handleSelectItem(item);
				}}
			>
				<Unplug class="size-4" />
			</button>
		</div>
	</div>
{/snippet}

{#snippet serverContent()}
	{@const name = getCurrentName()}
	{@const icon = getCurrentIcon()}
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		<div class="flex flex-wrap items-center">
			<ChevronLeft class="mr-2 size-4" />
			<button
				onclick={() => (showServerInfo = false)}
				class="button-text flex items-center gap-2 p-0 text-lg font-light"
			>
				MCP Servers
			</button>
			<ChevronLeft class="mx-2 size-4" />
			<span class="text-lg font-light">{getCurrentName()}</span>
		</div>

		<div class="flex items-center gap-2">
			{#if icon}
				<img src={icon} alt={name} class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
			{:else}
				<Server class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
			{/if}
			<h1 class="text-2xl font-semibold capitalize">
				{getCurrentName()}
			</h1>
			<div class="flex grow items-center justify-end gap-4">
				{#if connectToEntry || (connectToServer && !connectToServer.connectURL)}
					<button
						class="button-primary"
						onclick={() => {
							if (connectToEntry) {
								if (hasEditableConfiguration(connectToEntry.entry)) {
									configDialog?.open();
								} else {
									handleLaunch();
								}
							} else if (connectToServer) {
								handleLaunch();
							}
						}}
					>
						Connect To Server
					</button>
				{:else if connectToServer && connectToServer.connectURL}
					<button
						class="button-primary"
						onclick={() => {
							connectDialog?.open();
						}}
					>
						Get Connection URL
					</button>
					<DotDotDot class="icon-button h size-10 min-h-auto min-w-auto flex-shrink-0 p-1">
						<div class="default-dialog flex min-w-48 flex-col p-2">
							{#if connectToServer.parent && hasEditableConfiguration(connectToServer.parent)}
								<button
									class="menu-button"
									onclick={async () => {
										if (!connectToServer?.server) {
											console.error('No user configured server for this entry found');
											return;
										}

										let values: Record<string, string>;
										try {
											values = await ChatService.revealSingleOrRemoteMcpServer(
												connectToServer.server.id
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
											id: connectToServer.server.id,
											envs: connectToServer.server.manifest.env?.map((env) => ({
												...env,
												value: values[env.key] ?? ''
											})),
											headers: connectToServer.server.manifest.headers?.map((header) => ({
												...header,
												value: values[header.key] ?? ''
											})),
											url: connectToServer.server.manifest.url,
											icon: connectToServer.server.manifest.icon,
											name: connectToServer.server.manifest.name
										};
										editUserConfiguredServerDialog?.open();
									}}
								>
									Edit Configuration
								</button>
							{/if}
							<button
								class="menu-button justify-between"
								onclick={() => {
									if (!connectToServer) return;
									handleSetupChat(connectToServer);
								}}
							>
								Chat <ExternalLink class="size-4 -translate-y-[1px]" />
							</button>
							<button
								class="menu-button text-red-500"
								onclick={async () => {
									if (!connectToServer) {
										console.error('No server to disconnect from');
										return;
									}

									if (connectToServer.instance) {
										deletingInstance = connectToServer.instance;
									} else if (connectToServer.parent) {
										deletingServer = connectToServer.server;
									}
								}}
							>
								Disconnect
							</button>
						</div>
					</DotDotDot>
				{/if}
			</div>
		</div>

		{#if connectToEntry}
			<McpServerInfo entry={connectToEntry.entry as MCPCatalogEntry} />
		{/if}

		{#if connectToServer?.server}
			<McpServerInfo entry={connectToServer.server} />
		{/if}
	</div>
{/snippet}

{#snippet connectUrlButton(url: string, name: string)}
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

	<HowToConnect servers={[{ url, name }]} />
{/snippet}

<ResponsiveDialog bind:this={configDialog} animate="slide" onClose={clearHighlights}>
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry}
		{#if connectToEntry.launching}
			<div class="my-4 flex flex-col justify-center gap-4"></div>
		{:else}
			{@render configureForm(connectToEntry, connectToEntry.entry)}
			<div class="flex justify-end">
				<button
					class={`button-primary ${
						connectToEntry.envs?.some((env) => env.required && !env.value) ||
						connectToEntry.headers?.some((header) => header.required && !header.value) ||
						(connectToEntry.entry.urlManifest && !connectToEntry.url)
							? 'cursor-not-allowed opacity-50'
							: ''
					}`}
					onclick={(e) => {
						if (!connectToEntry) return;

						const isDisabled =
							connectToEntry.envs?.some((env) => env.required && !env.value) ||
							connectToEntry.headers?.some((header) => header.required && !header.value) ||
							(connectToEntry.entry.urlManifest && !connectToEntry.url);

						if (isDisabled) {
							e.preventDefault();
							highlightMissingRequiredFields();
						} else {
							handleLaunch();
						}
					}}
				>
					Launch
				</button>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={editUserConfiguredServerDialog}
	onClose={() => {
		userConfiguredServerToEdit = undefined;
		clearHighlights();
	}}
>
	{#snippet titleContent()}
		<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
			{#if userConfiguredServerToEdit?.icon}
				<img
					src={userConfiguredServerToEdit.icon}
					alt={userConfiguredServerToEdit.name}
					class="size-8"
				/>
			{:else}
				<Server class="size-8" />
			{/if}
		</div>
		{userConfiguredServerToEdit?.name}
	{/snippet}
	{#if userConfiguredServerToEdit}
		{@render configureForm(userConfiguredServerToEdit)}
		<div class="flex justify-end">
			<button
				class={`button-primary ${
					userConfiguredServerToEdit?.envs?.some((env) => env.required && !env.value) ||
					userConfiguredServerToEdit?.headers?.some((header) => header.required && !header.value)
						? 'cursor-not-allowed opacity-50'
						: ''
				}`}
				onclick={async (e) => {
					if (!userConfiguredServerToEdit) return;

					const isDisabled =
						userConfiguredServerToEdit.envs?.some((env) => env.required && !env.value) ||
						userConfiguredServerToEdit.headers?.some((header) => header.required && !header.value);

					if (isDisabled) {
						e.preventDefault();
						// Highlight missing fields for edit dialog
						const fieldsToHighlight = new Set<string>();

						userConfiguredServerToEdit.envs?.forEach((env) => {
							if (env.required && !env.value) {
								fieldsToHighlight.add(env.key);
							}
						});

						userConfiguredServerToEdit.headers?.forEach((header) => {
							if (header.required && !header.value) {
								fieldsToHighlight.add(header.key);
							}
						});

						highlightedFields = fieldsToHighlight;
					} else {
						const secretValues = convertEnvHeadersToRecord(
							userConfiguredServerToEdit.envs,
							userConfiguredServerToEdit.headers
						);
						await ChatService.configureSingleOrRemoteMcpServer(
							userConfiguredServerToEdit.id,
							secretValues
						);
						clearHighlights();
						editUserConfiguredServerDialog?.close();
						await loadData(true);
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
			{#each fields.envs as env, i (env.key)}
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
					<div class={highlightedFields.has(env.key) ? 'rounded ring-2 ring-red-500' : ''}>
						{#if env.sensitive}
							<SensitiveInput name={env.name} bind:value={fields.envs[i].value} />
						{:else}
							<input
								type="text"
								id={env.key}
								bind:value={fields.envs[i].value}
								class="text-input-filled"
								oninput={() => {
									if (highlightedFields.has(env.key)) {
										highlightedFields.delete(env.key);
										highlightedFields = new Set(highlightedFields);
									}
								}}
							/>
						{/if}
					</div>
				</div>
			{/each}
		{/if}
		{#if fields.headers && fields.headers.length > 0}
			{#each fields.headers as header, i (header.key)}
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
					<div class={highlightedFields.has(header.key) ? 'rounded ring-2 ring-red-500' : ''}>
						{#if header.sensitive}
							<SensitiveInput name={header.name} bind:value={fields.headers[i].value} />
						{:else}
							<input
								type="text"
								id={header.key}
								bind:value={fields.headers[i].value}
								class="text-input-filled"
								oninput={() => {
									if (highlightedFields.has(header.key)) {
										highlightedFields.delete(header.key);
										highlightedFields = new Set(highlightedFields);
									}
								}}
							/>
						{/if}
					</div>
				</div>
			{/each}
		{/if}
		{#if entry?.urlManifest || fields.url}
			<div class="flex flex-col gap-1">
				<label for="url-manifest-url"> URL </label>
				<div class={highlightedFields.has('url-manifest-url') ? 'rounded ring-2 ring-red-500' : ''}>
					<input
						type="text"
						id="url-manifest-url"
						bind:value={fields.url}
						class="text-input-filled"
						oninput={() => {
							if (highlightedFields.has('url-manifest-url')) {
								highlightedFields.delete('url-manifest-url');
								highlightedFields = new Set(highlightedFields);
							}
						}}
					/>
				</div>
				{#if entry?.urlManifest?.hostname}
					<span class="font-light text-gray-400 dark:text-gray-600">
						The URL must contain the hostname: <b class="font-semibold">
							{entry.urlManifest.hostname}
						</b>
					</span>
				{/if}
			</div>
		{/if}
	</div>
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		{@render title()}
	{/snippet}

	{#if connectToEntry?.connectURL}
		{@render connectUrlButton(
			connectToEntry.connectURL,
			connectToEntry.entry.commandManifest?.name ?? connectToEntry.entry.urlManifest?.name ?? ''
		)}
	{:else if connectToServer?.connectURL}
		{@render connectUrlButton(
			connectToServer.connectURL,
			connectToServer.server?.manifest.name ?? ''
		)}
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
		servers={connectedServers.map((server) => ({
			url: server.connectURL ?? '',
			name: server.server?.manifest.name ?? ''
		}))}
	/>
</ResponsiveDialog>

{#snippet title()}
	{#if connectToEntry}
		{@const name =
			connectToEntry.entry.commandManifest?.name ?? connectToEntry.entry.urlManifest?.name}
		{@const icon =
			connectToEntry.entry.commandManifest?.icon ?? connectToEntry.entry.urlManifest?.icon}

		<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
			{#if icon}
				<img src={icon} alt={name} class="size-8" />
			{:else}
				<Server class="size-8" />
			{/if}
		</div>
		{name}
	{:else if connectToServer}
		<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
			{#if connectToServer.server?.manifest.icon}
				<img
					src={connectToServer.server?.manifest.icon}
					alt={connectToServer.server?.manifest.name}
					class="size-8"
				/>
			{:else}
				<Server class="size-8" />
			{/if}
		</div>
		{connectToServer.server?.manifest.name}
	{/if}
{/snippet}

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingInstance)}
	onsuccess={async () => {
		if (deletingInstance) {
			await ChatService.deleteMcpServerInstance(deletingInstance.id);
			await loadData(true);
			deletingInstance = undefined;
			showServerInfo = false;
			connectToServer = undefined;
		}
	}}
	oncancel={() => (deletingInstance = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (deletingServer) {
			await ChatService.deleteSingleOrRemoteMcpServer(deletingServer.id);
			await loadData(true);
			deletingServer = undefined;
			showServerInfo = false;
			connectToServer = undefined;
		}
	}}
	oncancel={() => (deletingServer = undefined)}
/>

<PageLoading show={chatLoading} text="Loading chat..." />

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
