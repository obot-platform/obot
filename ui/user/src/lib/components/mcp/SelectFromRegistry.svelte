<script lang="ts">
	/**
	 * TODO: Change the logic in here to represent connecting to a server template
	 */
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord,
		hasEditableConfiguration,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { fly } from 'svelte/transition';
	import type { LaunchFormData, CompositeLaunchFormData } from './CatalogConfigureForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChevronLeft, ChevronRight, Info, LoaderCircle, ServerIcon } from 'lucide-svelte';
	import { tick, type Snippet } from 'svelte';
	import McpCard from './McpCard.svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import CatalogConfigureForm from './CatalogConfigureForm.svelte';
	import CatalogEditAliasForm from './CatalogEditAliasForm.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import Confirm from '../Confirm.svelte';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfoAndTools from './McpServerInfoAndTools.svelte';
	import PageLoading from '../PageLoading.svelte';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';

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
		classes?: {
			pageSelectorContainer?: string;
		};
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
		classes,
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
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let showServerInfo = $state(false);
	let editingServer = $state<ConnectedServer>();
	let editAliasDialog = $state<ReturnType<typeof CatalogEditAliasForm>>();

	let selectedEntryOrServer = $state<Entry | ConnectedServer | Server>();
	let selectedManifest = $derived(getManifest(selectedEntryOrServer));
	let search = $state('');
	let saving = $state(false);
	let updating = $state(false);
	let error = $state<string>();

	let launching = $state(false);
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let relaunching = $state(false);

	let deletingInstance = $state<MCPServerInstance>();
	let deletingServer = $state<MCPCatalogServer>();

	let serverInstancesMap = $derived(
		new Map(
			userServerInstances
				.filter((instance) => !instance.deleted)
				.map((instance) => [(instance.mcpServerID ?? instance.mcpCatalogID) as string, instance])
		)
	);

	let filteredEntriesData = $derived(
		entries.filter((item) => {
			if (item.deleted) {
				return false;
			}

			if (selectedCategory && !item.categories.includes(selectedCategory)) {
				return false;
			}

			if (search) {
				const nameToUse = item.manifest?.name;
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

	let filteredData = $derived(
		[...filteredServers, ...filteredEntriesData].sort((a, b) => {
			if (a.manifest?.name && b.manifest?.name) {
				return a.manifest.name.localeCompare(b.manifest.name);
			}
			return 0;
		})
	);
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
	let userServerConfigureMap = $derived(
		new Set(
			userConfiguredServers
				.filter((server) => !server.deleted)
				.map((server) => server.catalogEntryID)
		)
	);
	let filteredConnectedServers = $derived(
		connectedServers
			.filter((item) => {
				if (search) {
					const searchLower = search.toLowerCase();
					const manifestName = item.server?.manifest.name?.toLowerCase() || '';
					const alias = item.server?.alias?.toLowerCase() || '';
					return manifestName.includes(searchLower) || alias.includes(searchLower);
				}
				return true;
			})
			.sort((a, b) => {
				if (a.server?.manifest.name && b.server?.manifest.name) {
					return a.server.manifest.name.localeCompare(b.server.manifest.name);
				}
				return 0;
			})
	);

	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

	function getUniqueAlias(serverName: string): string | undefined {
		const existingNames = userConfiguredServers
			.flatMap((server) => [server.manifest?.name || '', server.alias || ''])
			.filter(Boolean)
			.map((name) => name.toLowerCase());

		const nameLower = serverName.toLowerCase();

		// Return undefined if no conflict
		if (!existingNames.includes(nameLower)) {
			return undefined;
		}

		// Generate unique alias with counter
		let counter = 1;
		let candidateAlias: string;
		do {
			candidateAlias = `${serverName} ${counter}`;
			counter++;
		} while (existingNames.includes(candidateAlias.toLowerCase()));

		return candidateAlias;
	}

	export function reset() {
		page = 0;
		showServerInfo = false;
		selectedEntryOrServer = undefined;
		configureForm = undefined;
		deletingInstance = undefined;
		deletingServer = undefined;
	}

	function listLaunchLogs(mcpServerId: string) {
		launchLogsEventStream = new EventStreamService<string>();
		launchLogsEventStream.connect(`/api/mcp-servers/${mcpServerId}/logs`, {
			onMessage: (data) => {
				launchLogs = [...launchLogs, data];
			}
		});
	}

	function initUpdatingOrLaunchProgress(existing?: boolean) {
		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;
		if (existing) {
			updating = true;
		} else {
			launching = true;
		}

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		return { timeout1, timeout2, timeout3 };
	}

	async function handleLaunchCatalogEntry(entry: Entry, _retryingServer?: MCPCatalogServer) {
		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		const url =
			entry.manifest.runtime === 'remote'
				? (
						(configureForm as LaunchFormData | undefined)?.url ||
						entry.manifest.remoteConfig?.fixedURL
					)?.trim()
				: undefined;
		const serverName = entry.manifest.name || '';

		// Generate unique alias if there's a naming conflict
		const aliasToUse = configureForm?.name || getUniqueAlias(serverName);

		let response: MCPCatalogServer | undefined = undefined;
		try {
			response = await ChatService.createSingleOrRemoteMcpServer({
				catalogEntryID: entry.id,
				manifest: url ? { remoteConfig: { url } } : {},
				alias: aliasToUse
			});
		} catch (err) {
			console.error('error: ', err);
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		}

		if (response) {
			try {
				const lf = configureForm as LaunchFormData | undefined;
				const envs = convertEnvHeadersToRecord(lf?.envs, lf?.headers);
				const configuredResponse = await ChatService.configureSingleOrRemoteMcpServer(
					response.id,
					envs
				);

				const launchResponse = await ChatService.validateSingleOrRemoteMcpServerLaunched(
					configuredResponse.id
				);
				if (!launchResponse.success) {
					launchError = launchResponse.message;
					listLaunchLogs(configuredResponse.id);
				} else {
					launchProgress = 100;
				}

				selectedEntryOrServer = {
					server: configuredResponse,
					connectURL: configuredResponse.connectURL,
					instance: undefined,
					parent: entry
				} as ConnectedServer;

				if (!launchError) {
					const ref = selectedEntryOrServer;
					setTimeout(() => {
						launching = false;
						launchProgress = 0;
						onConnectServer?.(ref);
					}, 1000);
				}
			} catch (err) {
				launchError = err instanceof Error ? err.message : 'An unknown error occurred';
			} finally {
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);

				relaunching = false;
			}
		}
	}

	async function handleLaunchCompositeServer(entry: Entry) {
		// If no configureForm yet, initialize the composite form so user can enable/disable components.
		if (!configureForm || !('componentConfigs' in configureForm)) {
			initCompositeForm(entry);
			return;
		}

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;
		launching = true;

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		try {
			const entry = selectedEntryOrServer as Entry;
			const aliasToUse =
				(configureForm as { name?: string } | undefined)?.name ||
				getUniqueAlias(selectedManifest?.name || '');
			const componentServersForCreate: Array<{
				catalogEntryID: string;
				manifest: Record<string, unknown>;
			}> = [];
			const payload: Record<
				string,
				{ config: Record<string, string>; url?: string; disabled?: boolean }
			> = {};
			for (const [id, comp] of Object.entries(configureForm.componentConfigs)) {
				const url = comp.url?.trim();
				componentServersForCreate.push({
					catalogEntryID: id,
					manifest: url
						? { remoteConfig: { url: url.startsWith('http') ? url : `https://${url}` } }
						: {}
				});
				const config: Record<string, string> = {};
				for (const f of [
					...(comp.envs ?? ([] as Array<{ key: string; value: string }>)),
					...(comp.headers ?? ([] as Array<{ key: string; value: string }>))
				]) {
					if (f.value) config[f.key] = f.value;
				}
				payload[id] = { config, url, disabled: comp.disabled ?? false };
			}

			const manifest: Record<string, unknown> = {
				compositeConfig: { componentServers: componentServersForCreate }
			};
			const created = await ChatService.createSingleOrRemoteMcpServer({
				catalogEntryID: entry.id,
				alias: aliasToUse,
				manifest
			});

			await ChatService.configureCompositeMcpServer(created.id, payload);

			const launchResponse = await ChatService.validateSingleOrRemoteMcpServerLaunched(created.id);
			if (!launchResponse.success) {
				launchError = launchResponse.message;
			} else {
				launchProgress = 100;
			}

			selectedEntryOrServer = {
				server: created,
				connectURL: created.connectURL,
				instance: undefined,
				parent: entry
			} as ConnectedServer;

			if (!launchError) {
				const ref = selectedEntryOrServer;
				setTimeout(() => {
					launching = false;
					launchProgress = 0;
					onConnectServer?.(ref);
				}, 1000);
			}
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);

			relaunching = false;
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
			if (
				'isCatalogEntry' in selectedEntryOrServer &&
				selectedEntryOrServer.manifest?.runtime === 'composite'
			) {
				await handleLaunchCompositeServer(selectedEntryOrServer as Entry);
			} else if ('isCatalogEntry' in selectedEntryOrServer) {
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

		return (item as ConnectedServer).server?.manifest;
	}

	function isSingleOrRemote(connectedServer: ConnectedServer | undefined) {
		return connectedServer?.server && !connectedServer.instance;
	}

	function initConfigureForm(item: Entry) {
		configureForm = {
			name: '',
			envs: item.manifest?.env?.map((env) => ({
				...env,
				value: ''
			})),
			headers: item.manifest?.remoteConfig?.headers?.map((header) => ({
				...header,
				value: ''
			})),
			...(item.manifest?.remoteConfig?.hostname
				? { hostname: item.manifest.remoteConfig?.hostname, url: '' }
				: {})
		};
	}

	function initCompositeForm(entry: Entry) {
		// For composite: open form first to collect per-component URLs before creating
		if (entry.manifest.runtime === 'composite') {
			const components = entry.manifest?.compositeConfig?.componentServers || [];
			const componentConfigs: Record<
				string,
				{
					name?: string;
					icon?: string;
					hostname?: string;
					url?: string;
					disabled?: boolean;
					envs?: Array<Record<string, unknown> & { key: string; value: string }>;
					headers?: Array<Record<string, unknown> & { key: string; value: string }>;
				}
			> = {};
			for (const c of components) {
				const id = c.catalogEntryID || c.mcpServerID;
				if (!id || !c.manifest) continue;
				const m = c.manifest;
				componentConfigs[id] = {
					name: m.name,
					icon: m.icon,
					hostname: m.remoteConfig?.hostname,
					url: m.remoteConfig?.fixedURL ?? '',
					disabled: false,
					envs: (m.env ?? []).map((e) => ({
						...(e as unknown as Record<string, unknown>),
						key: e.key,
						value: ''
					})),
					headers: (m.remoteConfig?.headers ?? []).map((h) => ({
						...(h as unknown as Record<string, unknown>),
						key: h.key,
						value: ''
					}))
				};
			}
			configureForm = { componentConfigs } as CompositeLaunchFormData;
			configDialog?.open();
		}
	}

	async function updateExistingRemoteOrSingleUser(lf: LaunchFormData, server: ConnectedServer) {
		if (!server.server) return;
		if (
			server.parent &&
			server.parent.manifest.runtime === 'remote' &&
			server.parent.manifest.remoteConfig?.urlTemplate === undefined &&
			lf?.url
		) {
			await ChatService.updateRemoteMcpServerUrl(server.server.id, lf.url.trim());
		}

		const envs = convertEnvHeadersToRecord(lf.envs, lf.headers);
		await ChatService.configureSingleOrRemoteMcpServer(server.server.id, envs);

		const updatedServer = await ChatService.getSingleOrRemoteMcpServer(server.server.id);
		selectedEntryOrServer = {
			...selectedEntryOrServer,
			server: updatedServer
		} as ConnectedServer;
	}

	async function updateExistingComposite(lf: CompositeLaunchFormData, server: ConnectedServer) {
		if (!server.server) return;
		// Composite flow using CatalogConfigureForm data
		if ('componentConfigs' in lf) {
			const payload = convertCompositeLaunchFormDataToPayload(lf);
			await ChatService.configureCompositeMcpServer(server.server.id, payload);
		}
	}

	async function handleConfigureForm() {
		if (!selectedEntryOrServer) return;
		if (!configureForm) return;

		if (
			relaunching &&
			'parent' in selectedEntryOrServer &&
			'server' in selectedEntryOrServer &&
			selectedEntryOrServer.parent &&
			selectedEntryOrServer.server
		) {
			configDialog?.close();
			await handleLaunchCatalogEntry(selectedEntryOrServer.parent, selectedEntryOrServer.server);
			return;
		}

		try {
			if ('server' in selectedEntryOrServer && selectedEntryOrServer.server?.id) {
				configDialog?.close();
				const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress(true);
				// updating existing
				if (selectedEntryOrServer.parent?.manifest.runtime === 'composite') {
					const lf = configureForm as CompositeLaunchFormData;
					await updateExistingComposite(lf, selectedEntryOrServer);
				} else {
					const lf = configureForm as LaunchFormData;
					await updateExistingRemoteOrSingleUser(lf, selectedEntryOrServer);
				}
				launchProgress = 100;
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);
				onUpdateConfigure?.();

				setTimeout(() => {
					updating = false;
				}, 1000);
			} else {
				// launching new
				configDialog?.close();
				await new Promise((resolve) => setTimeout(resolve, 300));
				await handleLaunch();
			}
		} catch (_error) {
			console.error('Error during configuration:', _error);
			configDialog?.close();
		}
	}

	async function handleSelectCard(item: Entry | Server | ConnectedServer) {
		search = ''; // reset search
		showServerInfo = true;
		selectedEntryOrServer = item;
		await tick();
		document.getElementsByTagName('main')[0].scrollTo({ top: 0, behavior: 'instant' });
	}

	async function handleEditConfiguration(connectedServer: ConnectedServer) {
		if (!connectedServer?.server) {
			console.error('No user configured server for this entry found');
			return;
		}
		if (connectedServer.parent?.manifest.runtime === 'composite') {
			configureForm = await convertCompositeInfoToLaunchFormData(connectedServer.server);
			selectedEntryOrServer = connectedServer;
			configDialog?.open();
			return;
		}

		let values: Record<string, string>;
		try {
			values = await ChatService.revealSingleOrRemoteMcpServer(connectedServer.server.id, {
				dontLogErrors: true
			});
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
			headers: connectedServer.server.manifest.remoteConfig?.headers?.map((header) => ({
				...header,
				value: values[header.key] ?? ''
			})),
			url: connectedServer.server.manifest.remoteConfig?.url,
			hostname: connectedServer.parent?.manifest.remoteConfig?.hostname
		};
		configDialog?.open();
	}

	async function handleCancelLaunch() {
		if (launchLogsEventStream) {
			launchLogsEventStream.disconnect();
		}
		if (
			selectedEntryOrServer &&
			'server' in selectedEntryOrServer &&
			selectedEntryOrServer.server
		) {
			await ChatService.deleteSingleOrRemoteMcpServer(selectedEntryOrServer.server.id);
			selectedEntryOrServer = selectedEntryOrServer.parent;
		}

		launching = false;
		launchError = undefined;
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if !showServerInfo}
	<div
		class="flex h-full flex-col gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
		bind:this={container}
	>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-background shadow-sm dark:border"
				onChange={(val) => {
					search = val;
					page = 0;
				}}
				placeholder="Search by name..."
				value={search}
			/>

			{#if filteredConnectedServers.length > 0}
				<div class="flex flex-col gap-4">
					<div class="flex items-center gap-4">
						<h2 class="text-lg font-semibold">Enabled Connectors</h2>
						{#if appendConnectedServerTitle}
							{@render appendConnectedServerTitle()}
						{/if}
					</div>
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
						{#each filteredConnectedServers as connectedServer, i (i)}
							{#if connectedServer.server}
								<McpCard
									data={connectedServer.server}
									parent={connectedServer.parent}
									onClick={() => {
										if (onConnectedServerCardClick) {
											onConnectedServerCardClick(connectedServer);
										} else {
											handleSelectCard(connectedServer);
										}
									}}
								>
									{#snippet action()}
										<div class="flex items-center gap-1">
											{@render connectedServerCardAction?.(connectedServer)}
											<DotDotDot
												class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
												{disablePortal}
												el={container}
											>
												<div class="default-dialog flex min-w-48 flex-col p-2">
													{#if isSingleOrRemote(connectedServer)}
														{@render editConfigAction(connectedServer)}
														{@render renameAction(connectedServer)}
													{/if}
													{@render additConnectedServerCardActions?.(connectedServer)}
													{@render disconnectAction(connectedServer)}
												</div>
											</DotDotDot>
										</div>
									{/snippet}
								</McpCard>
							{/if}
						{/each}
					</div>
				</div>
			{/if}
			<div class="flex h-full flex-col gap-4">
				<h2 class="text-lg font-semibold">Available Connectors</h2>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
					{#each paginatedData as item (item.id)}
						<McpCard
							data={item}
							onClick={() => {
								handleSelectCard(item);
							}}
						/>
					{/each}
				</div>
				{#if filteredEntriesData.length > pageSize}
					<div
						class={twMerge(
							'bg-surface1 dark:bg-background sticky bottom-0 left-0 mt-auto flex w-[calc(100%+2em)] -translate-x-4 items-center justify-center gap-4 p-2 md:w-[calc(100%+4em)] md:-translate-x-8',
							classes?.pageSelectorContainer
						)}
					>
						<button
							class="button-text flex items-center gap-1"
							onclick={() => (page = page - 1)}
							disabled={page === 0}
						>
							<ChevronLeft class="size-4" /> Previous
						</button>
						<span class="text-on-surface1 text-sm">
							{page + 1} of {Math.ceil(filteredEntriesData.length / pageSize)}
						</span>
						<button
							class="button-text flex items-center gap-1"
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
	name={selectedEntryOrServer && 'server' in selectedEntryOrServer
		? selectedEntryOrServer.server?.alias || selectedManifest?.name
		: selectedManifest?.name}
	onSave={handleConfigureForm}
	submitText={selectedEntryOrServer && 'server' in selectedEntryOrServer ? 'Update' : 'Launch'}
	loading={saving}
	isNew={selectedEntryOrServer && 'isCatalogEntry' in selectedEntryOrServer}
	showAlias={selectedEntryOrServer &&
		'isCatalogEntry' in selectedEntryOrServer &&
		userServerConfigureMap.has(selectedEntryOrServer.id)}
/>

<CatalogEditAliasForm bind:this={editAliasDialog} {editingServer} {onUpdateConfigure} />

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

<PageLoading
	isProgressBar
	show={launching || updating}
	text={updating ? 'Updating server...' : 'Configuring and initializing server...'}
	progress={launchProgress}
	error={launchError}
	errorClasses={{
		root: 'md:w-[95vw]'
	}}
	onClose={handleCancelLaunch}
>
	{#snippet errorPreContent()}
		<h4 class="text-xl font-semibold">MCP Server Launch Failed</h4>
	{/snippet}
	{#snippet errorPostContent()}
		{@const hasConfigurableParent =
			selectedEntryOrServer &&
			'parent' in selectedEntryOrServer &&
			selectedEntryOrServer.parent &&
			hasEditableConfiguration(selectedEntryOrServer.parent)}
		{#if launchLogs.length > 0}
			<div
				class="default-scrollbar-thin bg-surface1 max-h-[50vh] w-full overflow-y-auto rounded-lg p-4 shadow-inner"
			>
				{#each launchLogs as log, i (i)}
					<div class="font-mono text-sm">
						<span class="text-on-surface1">{log}</span>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-md self-start">An issue occurred while launching the MCP server.</p>
		{/if}

		<div class="flex w-full flex-col items-center gap-2 md:flex-row">
			{#if hasConfigurableParent}
				<button
					class="button-primary w-full md:w-1/2 md:flex-1"
					onclick={() => {
						launching = false;
						launchError = undefined;
						relaunching = true;
						if (
							selectedEntryOrServer &&
							'parent' in selectedEntryOrServer &&
							selectedEntryOrServer.parent
						) {
							if (hasEditableConfiguration(selectedEntryOrServer.parent)) {
								configDialog?.open();
							} else {
								handleLaunch();
							}
						}
					}}
				>
					Update Configuration and Try Again
				</button>
			{/if}
			<button class="button w-full md:w-1/2 md:flex-1" onclick={handleCancelLaunch}>
				Cancel and Delete Server
			</button>
		</div>
	{/snippet}
</PageLoading>

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
				My Connectors
			</button>
			<ChevronLeft class="mx-2 size-4" />
			<span class="text-lg font-light"
				>{selectedEntryOrServer && 'server' in selectedEntryOrServer
					? selectedEntryOrServer.server?.alias || manifest?.name
					: manifest?.name}</span
			>
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
				{selectedEntryOrServer && 'server' in selectedEntryOrServer
					? selectedEntryOrServer.server?.alias || manifest?.name
					: manifest?.name}
			</h1>
			<div class="flex grow items-center justify-end gap-4">
				{#if !('server' in item)}
					<button
						disabled={saving}
						class="button-primary"
						onclick={() => {
							configureForm = undefined;
							if ('isCatalogEntry' in item) {
								if (hasEditableConfiguration(item) && item.manifest?.runtime === 'composite') {
									initCompositeForm(item);
								} else if (hasEditableConfiguration(item) || userServerConfigureMap.has(item.id)) {
									initConfigureForm(item);
									configDialog?.open();
								} else {
									handleLaunch();
								}
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
							{#if isSingleOrRemote(connectedServer)}
								{@render editConfigAction(connectedServer)}
							{/if}
							{@render additConnectedServerViewActions?.(connectedServer)}
							{@render disconnectAction(connectedServer)}
						</div>
					</DotDotDot>
				{/if}
			</div>
		</div>

		{#if 'isCatalogEntry' in item && userServerConfigureMap.has(item.id)}
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<p>
						It looks like you already have an existing server instance available. It is recommended
						to only create another one if you need to instantiate another one with different
						configurations.
					</p>
				</div>
			</div>
		{/if}

		{#if serverOrEntry}
			<McpServerInfoAndTools
				entry={serverOrEntry}
				parent={selectedEntryOrServer && 'parent' in selectedEntryOrServer
					? selectedEntryOrServer.parent
					: undefined}
				onEditConfiguration={() => {
					if (selectedEntryOrServer && 'parent' in selectedEntryOrServer) {
						handleEditConfiguration(selectedEntryOrServer);
					}
				}}
			/>
		{/if}
	</div>
{/snippet}

{#snippet editConfigAction(connectedServer: ConnectedServer)}
	{@const requiresUpdate = requiresUserUpdate(connectedServer)}
	{@const canConfigure =
		connectedServer.parent &&
		(connectedServer.parent.manifest.runtime === 'composite' ||
			hasEditableConfiguration(connectedServer.parent))}
	{#if canConfigure}
		<button
			class={twMerge(
				'menu-button',
				requiresUpdate && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
			)}
			onclick={() => handleEditConfiguration(connectedServer)}
		>
			Edit Configuration
		</button>
	{/if}
{/snippet}

{#snippet renameAction(connectedServer: ConnectedServer)}
	<button
		class="menu-button"
		onclick={() => {
			editingServer = connectedServer;
			editAliasDialog?.open();
		}}
	>
		Rename
	</button>
{/snippet}

{#snippet disconnectAction(connectedServer: ConnectedServer)}
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
