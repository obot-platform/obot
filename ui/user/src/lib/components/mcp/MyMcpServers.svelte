<script lang="ts">
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import {
		convertEnvHeadersToRecord,
		hasEditableConfiguration,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { fly } from 'svelte/transition';
	import type { LaunchFormData } from './CatalogConfigureForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChevronLeft, ChevronRight, LoaderCircle, ServerIcon } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import McpCard from './McpCard.svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import McpServerInfo from './McpServerInfo.svelte';
	import CatalogConfigureForm from './CatalogConfigureForm.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import Confirm from '../Confirm.svelte';
	import { twMerge } from 'tailwind-merge';

	type Entry = MCPCatalogEntry & {
		categories: string[]; // categories for the entry
	};

	type Server = MCPCatalogServer & {
		categories: string[]; // categories for the server
	};

	export type ConnectedServer = {
		connectURL: string;
		server?: Server;
		instance?: MCPServerInstance;
		parent?: Entry;
	};

	interface Props {
		userServerInstances: MCPServerInstance[]; // multi-user server instances
		userConfiguredServers: Server[]; // user servers created from single/remote servers
		servers: Server[]; // multi-user servers user has access to
		entries: Entry[]; // single-user servers user has access to
		loading: boolean;
		selectedCategory?: string;
		appendConnectedServerTitle?: Snippet;
		connectedServerCardAction?: Snippet<[ConnectedServer]>;
		additConnectedServerCardActions?: Snippet<[ConnectedServer]>;
		additConnectedServerViewActions?: Snippet<[ConnectedServer]>;
		onConnectedServerCardClick?: (connectedServer: ConnectedServer) => void;
		onConnectServer: (connectedServer: ConnectedServer) => void;
		onSelectConnectedServer?: (connectedServer: ConnectedServer) => void;
		onDisconnect?: () => void;
		onUpdateConfigure?: () => void;
		connectSelectText: string;
		disablePortal?: boolean;
	}

	let {
		userServerInstances,
		userConfiguredServers,
		servers,
		entries,
		loading,
		selectedCategory,
		appendConnectedServerTitle,
		additConnectedServerCardActions,
		additConnectedServerViewActions,
		connectedServerCardAction,
		onConnectedServerCardClick,
		onConnectServer,
		onSelectConnectedServer,
		onDisconnect,
		onUpdateConfigure,
		connectSelectText,
		disablePortal
	}: Props = $props();

	let container = $state<HTMLDivElement>();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configureForm = $state<LaunchFormData>();
	let showServerInfo = $state(false);

	let selectedEntryOrServer = $state<Entry | ConnectedServer | Server>();
	let selectedManifest = $derived(getManifest(selectedEntryOrServer));
	let search = $state('');
	let saving = $state(false);
	let error = $state<string>();

	let deletingInstance = $state<MCPServerInstance>();
	let deletingServer = $state<MCPCatalogServer>();

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

			if (selectedCategory && !item.categories.includes(selectedCategory)) {
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

			if (selectedCategory && !item.categories.includes(selectedCategory)) {
				return false;
			}

			if (search) {
				return item.manifest.name?.toLowerCase().includes(search.toLowerCase());
			}

			return true;
		})
	);

	let filteredData = $derived([...filteredServers, ...filteredEntriesData]);
	let connectedServers: ConnectedServer[] = $derived([
		...userConfiguredServers
			.filter(
				(server) =>
					server.connectURL &&
					!server.deleted &&
					(!selectedCategory || server.categories.includes(selectedCategory))
			)
			.map((server) => ({
				connectURL: server.connectURL ?? '',
				server,
				instance: undefined,
				parent: server.catalogEntryID
					? (entries.find((e) => e.id === server.catalogEntryID) ?? undefined)
					: undefined
			})),
		...userServerInstances
			.map((instance) => ({
				connectURL: instance.connectURL ?? '',
				instance,
				server: servers.find((s) => s.id === instance.mcpServerID) ?? undefined,
				parent: undefined
			}))
			.filter((item) => !selectedCategory || item.server?.categories?.includes(selectedCategory))
	]);
	let filteredConnectedServers = $derived(
		connectedServers.filter((item) => {
			if (search) {
				return item.server?.manifest.name?.toLowerCase().includes(search.toLowerCase());
			}
			return true;
		})
	);

	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

	export function reset() {
		page = 0;
		showServerInfo = false;
		selectedEntryOrServer = undefined;
		configureForm = undefined;
		deletingInstance = undefined;
		deletingServer = undefined;
	}
	async function handleLaunchCatalogEntry(entry: Entry) {
		const manifest = entry.commandManifest ?? entry.urlManifest;
		if (!manifest) {
			console.error('No server manifest found');
			return;
		}

		const url = configureForm?.url;
		try {
			const response = await ChatService.createSingleOrRemoteMcpServer({
				catalogEntryID: entry.id,
				...(entry.urlManifest ? { manifest: { url } } : {})
			});
			const secretValues = convertEnvHeadersToRecord(configureForm?.envs, configureForm?.headers);
			const configuredResponse = await ChatService.configureSingleOrRemoteMcpServer(
				response.id,
				secretValues
			);
			selectedEntryOrServer = {
				server: configuredResponse,
				connectURL: configuredResponse.connectURL,
				instance: undefined,
				parent: entry
			} as ConnectedServer;

			onConnectServer?.(selectedEntryOrServer);
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred';
		}
	}

	async function handleMultiUserServer(server: Server) {
		try {
			const instance = await ChatService.createMcpServerInstance(server.id);
			selectedEntryOrServer = {
				server,
				connectURL: instance.connectURL,
				instance,
				parent: undefined
			} as ConnectedServer;
			onConnectServer?.(selectedEntryOrServer);
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred';
		}
	}

	async function handleLaunch() {
		if (!selectedEntryOrServer) return;
		if ('server' in selectedEntryOrServer) return; // connected server doesn't need to launch again

		error = undefined;
		saving = true;
		try {
			const isCatalogEntry =
				'urlManifest' in selectedEntryOrServer || 'commandManifest' in selectedEntryOrServer;
			if (isCatalogEntry) {
				await handleLaunchCatalogEntry(selectedEntryOrServer as Entry);
			} else {
				await handleMultiUserServer(selectedEntryOrServer as Server);
			}
		} catch (error) {
			console.error('Error during launching', error);
		} finally {
			saving = false;
		}
	}

	function getManifest(item?: Entry | Server | ConnectedServer) {
		if (!item) return undefined;

		if ('manifest' in item) {
			return item.manifest;
		}

		if ('commandManifest' in item || 'urlManifest' in item) {
			return item.commandManifest ?? item.urlManifest;
		}

		return (item as ConnectedServer).server?.manifest;
	}

	function initConfigureForm(item: Entry) {
		const manifest = item.commandManifest ?? item.urlManifest;
		configureForm = {
			envs: manifest?.env?.map((env) => ({
				...env,
				value: ''
			})),
			headers: manifest?.headers?.map((header) => ({
				...header,
				value: ''
			})),
			...(manifest?.hostname ? { hostname: manifest.hostname, url: '' } : {})
		};
	}

	async function handleConfigureForm() {
		if (!selectedEntryOrServer) return;
		if (!configureForm) return;

		try {
			if ('server' in selectedEntryOrServer && selectedEntryOrServer.server?.id) {
				if (selectedEntryOrServer.parent && selectedEntryOrServer.parent.urlManifest) {
					await ChatService.updateSingleOrRemoteMcpServer(selectedEntryOrServer.server.id, {
						...selectedEntryOrServer.parent.urlManifest,
						url: configureForm.url
					});
				}

				const secretValues = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
				await ChatService.configureSingleOrRemoteMcpServer(
					selectedEntryOrServer.server.id,
					secretValues
				);

				configDialog?.close();
				onUpdateConfigure?.();
			} else {
				configDialog?.close();
				// Add a small delay to ensure dialog is fully closed before handling launch
				await new Promise((resolve) => setTimeout(resolve, 300));
				await handleLaunch();
			}
		} catch (error) {
			console.error('Error during configuration:', error);
			configDialog?.close();
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if !showServerInfo}
	<div
		class="flex flex-col gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
		bind:this={container}
	>
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

			{#if filteredConnectedServers.length > 0}
				<div class="flex flex-col gap-4">
					<div class="flex items-center gap-4">
						<h2 class="text-lg font-semibold">Connected MCP Servers</h2>
						{#if appendConnectedServerTitle}
							{@render appendConnectedServerTitle()}
						{/if}
					</div>
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
						{#each filteredConnectedServers as connectedServer, i (i)}
							{#if connectedServer.server}
								<McpCard
									data={connectedServer.server}
									onClick={() => {
										if (onConnectedServerCardClick) {
											onConnectedServerCardClick(connectedServer);
										} else {
											showServerInfo = true;
											selectedEntryOrServer = connectedServer;
										}
									}}
								>
									{#snippet action()}
										{#if connectedServerCardAction}
											{@render connectedServerCardAction(connectedServer)}
										{:else}
											<DotDotDot
												class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
												{disablePortal}
												el={container}
											>
												<div class="default-dialog flex min-w-48 flex-col p-2">
													{@render prependedDefaultActions(connectedServer)}
													{#if additConnectedServerCardActions}
														{@render additConnectedServerCardActions(connectedServer)}
													{/if}
													{@render appendedDefaultActions(connectedServer)}
												</div>
											</DotDotDot>
										{/if}
									{/snippet}
								</McpCard>
							{/if}
						{/each}
					</div>
				</div>
			{/if}
			<div class="flex flex-col gap-4">
				<h2 class="text-lg font-semibold">Available MCP Servers</h2>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
					{#each paginatedData as item (item.id)}
						<McpCard
							data={item}
							onClick={() => {
								showServerInfo = true;
								selectedEntryOrServer = item;
							}}
						/>
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
{:else if showServerInfo && selectedEntryOrServer}
	{@render serverInfo(selectedEntryOrServer)}
{/if}

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={selectedManifest?.icon}
	name={selectedManifest?.name}
	onSave={handleConfigureForm}
	submitText={selectedEntryOrServer && 'server' in selectedEntryOrServer ? 'Update' : 'Launch'}
	loading={saving}
/>

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingInstance)}
	onsuccess={async () => {
		if (deletingInstance) {
			await ChatService.deleteMcpServerInstance(deletingInstance.id);
			reset();
			onDisconnect?.();
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
			reset();
			onDisconnect?.();
		}
	}}
	oncancel={() => (deletingServer = undefined)}
/>

{#snippet serverInfo(item: Entry | Server | ConnectedServer)}
	{@const manifest = getManifest(item)}
	{@const serverOrEntry = item
		? 'server' in item
			? item.server
			: (item as Entry | Server)
		: undefined}
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		<div class="flex flex-wrap items-center">
			<ChevronLeft class="mr-2 size-4" />
			<button
				onclick={() => {
					selectedEntryOrServer = undefined;
					showServerInfo = false;
				}}
				class="button-text flex items-center gap-2 p-0 text-lg font-light"
			>
				MCP Servers
			</button>
			<ChevronLeft class="mx-2 size-4" />
			<span class="text-lg font-light">{manifest?.name}</span>
		</div>

		<div class="flex items-center gap-2">
			{#if manifest?.icon}
				<img
					src={manifest.icon}
					alt={manifest.name}
					class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
				/>
			{:else}
				<ServerIcon class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
			{/if}
			<h1 class="text-2xl font-semibold capitalize">
				{manifest?.name}
			</h1>
			<div class="flex grow items-center justify-end gap-4">
				{#if !('server' in item)}
					<button
						disabled={saving}
						class="button-primary"
						onclick={() => {
							configureForm = undefined;
							const isCatalogEntry = 'commandManifest' in item || 'urlManifest' in item;
							if (isCatalogEntry && hasEditableConfiguration(item)) {
								initConfigureForm(item);
								configDialog?.open();
							} else {
								handleLaunch();
							}
						}}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Connect To Server
						{/if}
					</button>
				{:else}
					{@const connectedServer = item as ConnectedServer}
					<button class="button-primary" onclick={() => onSelectConnectedServer?.(connectedServer)}>
						{connectSelectText}
					</button>
					<DotDotDot
						class="icon-button size-10 min-h-auto min-w-auto flex-shrink-0 p-1"
						{disablePortal}
					>
						<div class="default-dialog flex min-w-48 flex-col p-2">
							{@render prependedDefaultActions(connectedServer)}
							{@render additConnectedServerViewActions?.(connectedServer)}
							{@render appendedDefaultActions(connectedServer)}
						</div>
					</DotDotDot>
				{/if}
			</div>
		</div>

		{#if serverOrEntry}
			<McpServerInfo entry={serverOrEntry} />
		{/if}
	</div>
{/snippet}

{#snippet prependedDefaultActions(connectedServer: ConnectedServer)}
	{@const configured = requiresUserUpdate(connectedServer)}
	<button
		class={twMerge(
			'menu-button',
			!configured && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
		)}
		onclick={async () => {
			if (!connectedServer?.server) {
				console.error('No user configured server for this entry found');
				return;
			}
			let values: Record<string, string>;
			try {
				values = await ChatService.revealSingleOrRemoteMcpServer(connectedServer.server.id);
			} catch (error) {
				if (error instanceof Error && !error.message.includes('404')) {
					console.error('Failed to reveal user server values due to unexpected error', error);
				}
				values = {};
			}
			selectedEntryOrServer = connectedServer;
			configureForm = {
				envs: connectedServer.server.manifest.env?.map((env) => ({
					...env,
					value: values[env.key] ?? ''
				})),
				headers: connectedServer.server.manifest.headers?.map((header) => ({
					...header,
					value: values[header.key] ?? ''
				})),
				url: connectedServer.server.manifest.url,
				hostname: connectedServer.parent?.urlManifest?.hostname
			};
			configDialog?.open();
		}}
	>
		Edit Configuration
	</button>
{/snippet}

{#snippet appendedDefaultActions(connectedServer: ConnectedServer)}
	<button
		class="menu-button text-red-500"
		onclick={async () => {
			if (!connectedServer) return;
			if (connectedServer.instance) {
				deletingInstance = connectedServer.instance;
			} else if (connectedServer.parent) {
				deletingServer = connectedServer.server;
			}
		}}
	>
		Disconnect
	</button>
{/snippet}
