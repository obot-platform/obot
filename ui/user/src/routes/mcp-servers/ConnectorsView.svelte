<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		UserService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type MCPServerInstance
	} from '$lib/services';
	import {
		convertEntriesAndServersToTableData,
		hasEditableConfiguration,
		disconnectMcpServerUser,
		hasMissingSecretBindingConfig,
		isMultiUserServer,
		restartMcpServer,
		getMCPDisplayName,
		isMultiUserCatalogEntry,
		requiresUserUpdate
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import ResponsiveDialog from '../../lib/components/ResponsiveDialog.svelte';
	import EditExistingDeployment from '../../lib/components/mcp/EditExistingDeployment.svelte';
	import DebugOauthDialog from '../../lib/components/mcp/oauth/DebugOauthDialog.svelte';
	import IconButton from '../../lib/components/primitives/IconButton.svelte';
	import ConnectUrlRow from './ConnectUrlRow.svelte';
	import { CircleFadingArrowUp, Server, StepForward } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type ServerSelectMode =
		| 'connect'
		| 'rename'
		| 'edit'
		| 'disconnect'
		| 'server-details'
		| 'restart'
		| 'reauthenticate';

	interface Props {
		entity?: 'workspace' | 'catalog';
		id?: string;
		catalog?: MCPCatalog;
		noDataContent?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
		onConnect?: ({ instance }: { instance?: MCPServerInstance }) => void;
	}

	let {
		entity,
		id,
		catalog = $bindable(),
		noDataContent,
		query,
		onConnect,
		usersMap
	}: Props = $props();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let selectedConfiguredServers = $state<MCPCatalogServer[]>([]);
	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectServerMode = $state<ServerSelectMode>('connect');

	let debugOauthDialog = $state<ReturnType<typeof DebugOauthDialog>>();

	let instancesMap = $derived(
		new Map(
			mcpServersAndEntries.current.userInstances.map((instance) => [instance.mcpServerID, instance])
		)
	);

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry]))
	);

	let tableData = $derived(
		convertEntriesAndServersToTableData(
			profile.current.hasAdminAccess?.()
				? mcpServersAndEntries.current.entries.filter((e) => e.canConnect)
				: mcpServersAndEntries.current.entries,
			profile.current.hasAdminAccess?.()
				? mcpServersAndEntries.current.servers.filter((s) => s.canConnect)
				: mcpServersAndEntries.current.servers,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.userInstances
		).filter((d) => {
			const requiresOauth =
				d.data.manifest?.runtime === 'remote' &&
				d.data.manifest?.remoteConfig &&
				'staticOAuthRequired' in d.data.manifest.remoteConfig &&
				d.data.manifest?.remoteConfig?.staticOAuthRequired;

			if ('isCatalogEntry' in d.data && isMultiUserCatalogEntry(d.data)) {
				return false;
			}

			return (
				!requiresOauth ||
				(requiresOauth && 'isCatalogEntry' in d.data && d.data.oauthCredentialConfigured)
			);
		})
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});
		return query
			? sorted.filter((d) => d.name.toLowerCase().includes(query.toLowerCase()))
			: sorted;
	});

	function getConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return mcpServersAndEntries.current.userConfiguredServers.filter(
			(server) => server.catalogEntryID === entry.id
		);
	}

	function getUsableConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return getConfiguredServersForCatalogEntry(entry).filter(
			(server) =>
				!hasMissingSecretBindingConfig(
					server.manifest,
					server.missingRequiredEnvVars,
					server.missingRequiredHeader
				)
		);
	}

	async function reauthenticateServer(server: MCPCatalogServer) {
		await UserService.clearMcpServerOAuth(server.id);
		await connectToServerDialog?.authenticate(
			server,
			server.catalogEntryID ? entriesMap.get(server.catalogEntryID) : undefined
		);
		await mcpServersAndEntries.refreshAll();
	}

	async function restartServer(server: MCPCatalogServer) {
		await restartMcpServer(server, catalog?.id);
	}

	async function disconnectCurrentUser(server: MCPCatalogServer) {
		await disconnectMcpServerUser(server);
	}

	function handleShowSelectServerDialog(
		entry: MCPCatalogEntry,
		mode: ServerSelectMode = 'connect'
	) {
		const allServers =
			mode === 'connect'
				? getUsableConfiguredServersForCatalogEntry(entry)
				: getConfiguredServersForCatalogEntry(entry);
		selectedConfiguredServers = allServers;
		selectedEntry = entry;
		selectServerDialog?.open();
		selectServerMode = mode;
	}

	function handleConnectToServer({
		server,
		instance
	}: {
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
	}) {
		if (instance || server) {
			mcpServersAndEntries.refreshAll();
		}
		onConnect?.({ instance });
	}

	function handleSelect(data: MCPCatalogEntry | MCPCatalogServer, e: MouseEvent | KeyboardEvent) {
		const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
		const isCtrlClick = isTouchDevice ? false : e.metaKey || e.ctrlKey;

		let url = '';
		if ('isCatalogEntry' in data && isMultiUserCatalogEntry(data)) {
			url = `/mcp-servers/c/${data.id}`;
		} else {
			const matchingServers =
				'isCatalogEntry' in data ? getConfiguredServersForCatalogEntry(data) : [];
			if (matchingServers.length === 1) {
				url = `/mcp-servers/c/${data.id}/instance/${matchingServers[0].id}`;
			} else if (matchingServers.length > 1) {
				handleShowSelectServerDialog(data as MCPCatalogEntry, 'server-details');
			} else {
				url = 'isCatalogEntry' in data ? `/mcp-servers/c/${data.id}` : `/mcp-servers/s/${data.id}`;
			}
		}

		openUrl(url, isCtrlClick);
	}

	function isUserConfigurationRequired(
		d: (typeof filteredTableData)[number],
		userConfiguredServers: MCPCatalogServer[]
	): boolean {
		if ('isCatalogEntry' in d.data) {
			return userConfiguredServers.length > 0
				? userConfiguredServers.some((s) => !s.configured)
				: hasEditableConfiguration(d.data);
		}

		return !d.data.configured;
	}

	function handleConnect(d: MCPCatalogEntry | MCPCatalogServer) {
		let instance: MCPServerInstance | undefined;
		let server: MCPCatalogServer | undefined;
		let entry: MCPCatalogEntry | undefined;

		if ('isCatalogEntry' in d) {
			if (isMultiUserCatalogEntry(d)) {
				const multiUserCatalogEntryServers = getConfiguredServersForCatalogEntry(d);
				if (multiUserCatalogEntryServers.length > 1) {
					handleShowSelectServerDialog(d);
				} else {
					server = multiUserCatalogEntryServers[0];
					instance = instancesMap.get(server.id);
					entry = d;
				}
			}

			const matchingServers = getUsableConfiguredServersForCatalogEntry(d);
			if (matchingServers.length > 1) {
				handleShowSelectServerDialog(d);
			} else {
				entry = d;
				server = matchingServers[0];
			}
		} else {
			const matchingEntry = d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined;
			instance = instancesMap.get(d.id);
			server = d;
			entry = matchingEntry;
		}

		if (server && !server.configured) {
			editExistingDialog?.edit({
				server,
				entry
			});
		} else {
			connectToServerDialog?.open({
				entry,
				server,
				instance
			});
		}
	}
</script>

<div class="flex flex-col gap-0.5 @container">
	{#if mcpServersAndEntries.current.loading}
		{#each Array.from({ length: 4 }) as _, i (i)}
			<div class="skeleton h-14 w-full"></div>
		{/each}
	{:else if tableData.length === 0 && noDataContent}
		{@render noDataContent?.()}
	{:else}
		{#each filteredTableData as d (d.id)}
			{@const isCatalogEntry = 'isCatalogEntry' in d.data}
			{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
			{@const userConfiguredServers = catalogEntry
				? getConfiguredServersForCatalogEntry(catalogEntry)
				: []}
			{@const instance = instancesMap.get(d.id)}
			{@const requiresUserConfiguration = isUserConfigurationRequired(d, userConfiguredServers)}
			{@const requiresUserAttention = instance
				? instance.configured === false
				: userConfiguredServers.some(requiresUserUpdate)}
			<div
				class={twMerge(
					'grid items-center grid-cols-12 rounded-md px-4 py-2 bg-base-100 dark:bg-base-300 shadow-xs hover:bg-base-300 dark:hover:bg-base-400 cursor-pointer'
				)}
				role="button"
				tabindex="0"
				onkeydown={(e) => {
					if (e.key === 'Enter' || e.key === ' ') {
						e.preventDefault();
						e.stopPropagation();
						handleSelect(d.data, e);
					}
				}}
				onclick={(e) => handleSelect(d.data, e)}
			>
				<div class="@2xl:col-span-4 col-span-7 text-sm font-light">
					<div class="flex items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
							{#if requiresUserAttention}
								<span
									use:tooltip={{
										classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
										text: 'An update requires your attention'
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</span>
							{/if}
						</p>
					</div>
				</div>
				<div class="@2xl:col-span-8 col-span-5">
					<div class="flex items-center gap-2 justify-end">
						<ConnectUrlRow
							connection={d.data}
							requiresConfiguration={requiresUserConfiguration}
							onClick={() => handleConnect(d.data)}
						/>
					</div>
				</div>
			</div>
		{/each}
	{/if}
</div>

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	catalogID={catalog?.id}
	workspaceID={entity === 'workspace' ? id : undefined}
	onConnect={handleConnectToServer}
/>

<ResponsiveDialog
	class="bg-base-200 dark:bg-base-100"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={async (d) => {
			selectServerDialog?.close();
			switch (selectServerMode) {
				case 'server-details': {
					goto(
						resolve(
							isMultiUserServer(d)
								? `/mcp-servers/s/${d.id}`
								: `/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`
						)
					);
					break;
				}
				case 'rename': {
					editExistingDialog?.rename({
						server: d,
						entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
					});
					break;
				}
				case 'edit': {
					editExistingDialog?.edit({
						server: d,
						entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
					});
					break;
				}
				case 'disconnect': {
					await disconnectCurrentUser(d);
					await mcpServersAndEntries.refreshAll();
					break;
				}
				case 'restart': {
					await restartServer(d);
					await mcpServersAndEntries.refreshAll();
					break;
				}
				case 'reauthenticate': {
					await reauthenticateServer(d);
					break;
				}
				default:
					connectToServerDialog?.open({
						entry: selectedEntry,
						server: d,
						instance: isMultiUserServer(d) ? instancesMap.get(d.id) : undefined
					});
					break;
			}
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{getMCPDisplayName(d)}
						{#if requiresUserUpdate(d)}
							<span
								use:tooltip={{
									classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
									text: 'An update requires your attention'
								}}
							>
								<CircleFadingArrowUp class="text-primary size-4" />
							</span>
						{/if}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4" />
			</IconButton>
		{/snippet}
	</Table>
</ResponsiveDialog>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={() => {
		mcpServersAndEntries.refreshUserConfiguredServers();
	}}
/>

<DebugOauthDialog bind:this={debugOauthDialog} />
