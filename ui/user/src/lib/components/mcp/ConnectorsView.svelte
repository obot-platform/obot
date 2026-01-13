<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import {
		AdminService,
		ChatService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		MCPCompositeDeletionDependencyError,
		Group,
		type MCPServerInstance
	} from '$lib/services';
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType,
		hasEditableConfiguration,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setSearchParamsToLocalStorage } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import {
		AlertTriangle,
		Captions,
		CircleFadingArrowUp,
		Ellipsis,
		LoaderCircle,
		MessageCircle,
		PencilLine,
		ReceiptText,
		SatelliteDish,
		Server,
		ServerCog,
		StepForward,
		Trash2,
		TriangleAlert,
		Unplug
	} from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type Item = ReturnType<typeof convertEntriesAndServersToTableData>[number];
	type ServerSelectMode = 'connect' | 'rename' | 'edit' | 'disconnect' | 'chat' | 'server-details';

	interface Props {
		entity?: 'workspace' | 'catalog';
		id?: string;
		catalog?: MCPCatalog;
		readonly?: boolean;
		noDataContent?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
		classes?: {
			tableHeader?: string;
		};
		onConnect?: ({ instance }: { instance?: MCPServerInstance }) => void;
	}

	let {
		entity,
		id,
		catalog = $bindable(),
		readonly,
		noDataContent,
		query,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort = { property: 'connected', order: 'desc' },
		classes,
		onConnect,
		usersMap
	}: Props = $props();

	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, Item>>({});
	let confirmBulkDelete = $state(false);
	let loadingBulkDelete = $state(false);
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let selectedConfiguredServers = $state<MCPCatalogServer[]>([]);
	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectServerMode = $state<ServerSelectMode>('connect');

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
			mcpServersAndEntries.current.entries,
			mcpServersAndEntries.current.servers,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.userInstances
		)
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});
		return query
			? sorted.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.registry.toLowerCase().includes(query.toLowerCase())
				)
			: sorted;
	});

	function getAuditLogsUrl(d: Item) {
		let useAdminUrl =
			window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();
		let hasAuditLogUrlsAccess = profile.current.groups.includes(Group.POWERUSER);

		if (!hasAuditLogUrlsAccess) {
			return null;
		}

		const isCatalogEntry = d.type === 'single' || d.type === 'remote' || d.type === 'composite';
		if (isCatalogEntry) {
			if (useAdminUrl) {
				return d.data.powerUserWorkspaceID
					? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${d.id}?view=audit-logs`
					: `/admin/mcp-servers/c/${d.id}?view=audit-logs`;
			}

			return `/mcp-servers/c/${d.id}?view=audit-logs`;
		}

		if (useAdminUrl) {
			return d.data.powerUserWorkspaceID
				? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${d.id}?view=audit-logs`
				: `/admin/mcp-servers/s/${d.id}?view=audit-logs`;
		}
		return `/mcp-servers/s/${d.id}?view=audit-logs`;
	}

	async function fetch() {
		mcpServersAndEntries.refreshAll();
	}

	function getConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return mcpServersAndEntries.current.userConfiguredServers.filter(
			(server) => server.catalogEntryID === entry.id
		);
	}

	function handleShowSelectServerDialog(
		entry: MCPCatalogEntry,
		mode: ServerSelectMode = 'connect'
	) {
		selectedConfiguredServers = getConfiguredServersForCatalogEntry(entry);
		selectedEntry = entry;
		selectServerDialog?.open();
		selectServerMode = mode;
	}
</script>

<div class="flex flex-col gap-2">
	{#if mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if mcpServersAndEntries.current.entries.length + mcpServersAndEntries.current.servers.length === 0}
		{#if noDataContent}
			{@render noDataContent?.()}
		{/if}
	{:else}
		{@const hasCatalogErrors = catalog && Object.keys(catalog?.syncErrors ?? {}).length > 0}
		{#if hasCatalogErrors && !catalog?.isSyncing}
			<div class="w-full p-4" in:slide={{ axis: 'y' }} out:slide={{ axis: 'y', duration: 0 }}>
				<div class="notification-alert flex w-full items-center gap-2 rounded-md p-3 text-sm">
					<AlertTriangle class="size-" />
					<p class="">Some servers failed to sync. See "Registry Sources" tab for more details.</p>
				</div>
			</div>
		{/if}

		<Table
			data={filteredTableData}
			fields={profile.current.hasAdminAccess?.()
				? ['name', 'connected', 'type', 'users', 'created', 'registry']
				: ['name', 'connected', 'created', 'registry']}
			headers={[{ title: 'Status', property: 'connected' }]}
			filterable={['name', 'type', 'registry']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				let url = '';
				const useAdminUrl =
					window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

				const matchedEntry =
					!('isCatalogEntry' in d.data) && d.data.catalogEntryID
						? entriesMap.get(d.data.catalogEntryID as string)
						: undefined;
				const powerUserWorkspaceID =
					matchedEntry?.powerUserWorkspaceID || d.data.powerUserWorkspaceID;
				if (useAdminUrl) {
					if ('isCatalogEntry' in d.data) {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/c/${d.data.id}`
							: `/admin/mcp-servers/c/${d.data.id}`;
					} else if (d.data.catalogEntryID) {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/c/${d.data.catalogEntryID}/instance/${d.id}`
							: `/admin/mcp-servers/c/${d.data.catalogEntryID}/instance/${d.id}`;
					} else {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/s/${d.id}`
							: `/admin/mcp-servers/s/${d.id}`;
					}
				} else {
					if ('isCatalogEntry' in d.data) {
						url = `/mcp-servers/c/${d.data.id}`;
					} else if (d.data.catalogEntryID) {
						url = `/mcp-servers/c/${d.data.catalogEntryID}/instance/${d.id}`;
					} else {
						url = `/mcp-servers/s/${d.id}`;
					}
				}

				setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
				openUrl(url, isCtrlClick);
			}}
			{initSort}
			{onFilter}
			{onClearAllFilters}
			{onSort}
			sortable={['name', 'connected', 'type', 'users', 'created', 'registry']}
			noDataMessage="No catalog servers added."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: classes?.tableHeader
			}}
			setRowClasses={(d) => {
				const matchingServers =
					'isCatalogEntry' in d.data ? getConfiguredServersForCatalogEntry(d.data) : [];
				return 'isCatalogEntry' in d.data && d.data.needsUpdate
					? 'bg-primary/10'
					: matchingServers.some(requiresUserUpdate)
						? 'bg-yellow-500/10'
						: '';
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{@const matchingServers =
					'isCatalogEntry' in d.data ? getConfiguredServersForCatalogEntry(d.data) : []}
				{#if property === 'name'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
							{#if 'isCatalogEntry' in d.data && d.data.needsUpdate}
								<span
									use:tooltip={{
										classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
										text: 'An update requires your attention'
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</span>
							{:else if matchingServers.some(requiresUserUpdate)}
								<span
									class="text-yellow-500"
									use:tooltip={{
										text: 'Server requires an update.'
									}}
								>
									<TriangleAlert class="size-4" />
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'connected'}
					{#if d.connected}
						<div class="pill-primary bg-primary">Connected</div>
					{/if}
				{:else if property === 'type'}
					{getServerTypeLabelByType(d.type)}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{@const auditLogUrl = getAuditLogsUrl(d)}
				{@const belongsToUser =
					(entity === 'workspace' && id && d.data.powerUserWorkspaceID === id) ||
					('catalogEntryID' in d.data && d.data.userID === profile.current.id)}
				{@const canDelete =
					d.editable && !readonly && (belongsToUser || profile.current?.hasAdminAccess?.())}
				{@const matchingServers =
					'isCatalogEntry' in d.data ? getConfiguredServersForCatalogEntry(d.data) : []}
				{@const matchingInstance =
					d.connected && d.type === 'multi' ? instancesMap.get(d.data.id) : undefined}
				{@const hasConnectedOptions =
					'isCatalogEntry' in d.data ? matchingServers.length > 0 : !!matchingInstance}
				<DotDotDot class="icon-button hover:dark:bg-background/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="default-dialog flex min-w-max flex-col">
							{#if hasConnectedOptions}
								<div
									class="bg-background dark:bg-surface2 rounded-t-xl p-2 pl-4 text-[11px] font-semibold uppercase"
								>
									My Connection(s)
								</div>
								<div class="bg-surface1 flex flex-col gap-1 p-2">
									{@render connectToServerAction(d.data, toggle)}
									<button
										class="menu-button hover:bg-surface3"
										onclick={async (e) => {
											e.stopPropagation();
											if ('isCatalogEntry' in d.data) {
												if (matchingServers.length === 1) {
													connectToServerDialog?.handleSetupChat(matchingServers[0]);
												} else {
													handleShowSelectServerDialog(d.data as MCPCatalogEntry, 'chat');
												}
											} else {
												connectToServerDialog?.handleSetupChat(d.data, instancesMap.get(d.id));
											}
											toggle(false);
										}}
									>
										<MessageCircle class="size-4" /> Chat
									</button>

									{#if 'isCatalogEntry' in d.data}
										{@render editCatalogEntryAction(d.data, matchingServers)}
										{@render renameCatalogEntryAction(d.data, matchingServers)}
									{/if}

									{#if matchingServers.length > 0}
										<button
											class="menu-button hover:bg-surface3"
											onclick={async (e) => {
												e.stopPropagation();
												if (matchingServers.length === 1) {
													goto(
														resolve(`/mcp-servers/c/${d.data.id}/instance/${matchingServers[0].id}`)
													);
												} else {
													handleShowSelectServerDialog(d.data as MCPCatalogEntry, 'server-details');
												}
												toggle(false);
											}}
										>
											<ReceiptText class="size-4" /> Server Details
										</button>
									{/if}

									{#if matchingServers.length > 0 && 'isCatalogEntry' in d.data}
										<button
											class="menu-button hover:bg-surface3"
											onclick={async (e) => {
												e.stopPropagation();

												if (matchingServers.length === 1) {
													await ChatService.deleteSingleOrRemoteMcpServer(matchingServers[0].id);
													mcpServersAndEntries.refreshUserConfiguredServers();
												} else {
													handleShowSelectServerDialog(d.data as MCPCatalogEntry, 'disconnect');
												}

												toggle(false);
											}}
										>
											<Unplug class="size-4" /> Disconnect
										</button>
									{:else if matchingInstance}
										<button
											class="menu-button hover:bg-surface3"
											onclick={async (e) => {
												e.stopPropagation();
												await ChatService.deleteMcpServerInstance(matchingInstance.id);
												mcpServersAndEntries.refreshUserInstances();
												toggle(false);
											}}
										>
											<Unplug class="size-4" /> Disconnect
										</button>
									{/if}
								</div>
							{/if}
							<div class="flex flex-col gap-1 p-2">
								{#if !hasConnectedOptions}
									{@render connectToServerAction(d.data, toggle, true)}
								{/if}
								{#if auditLogUrl && (belongsToUser || profile.current?.hasAdminAccess?.())}
									<button
										onclick={(e) => {
											e.stopPropagation();
											const isCtrlClick = e.ctrlKey || e.metaKey;
											setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
											openUrl(auditLogUrl, isCtrlClick);
										}}
										class="menu-button"
									>
										<Captions class="size-4" /> View Audit Logs
									</button>
								{/if}
								{#if canDelete}
									<button
										class="menu-button-destructive"
										onclick={(e) => {
											e.stopPropagation();
											if ('isCatalogEntry' in d.data) {
												deletingEntry = d.data;
											} else {
												deletingServer = d.data;
											}
											toggle(false);
										}}
									>
										<Trash2 class="size-4" /> Delete Entry
									</button>
								{/if}
							</div>
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
		</Table>
	{/if}
</div>

{#snippet editCatalogEntryAction(d: MCPCatalogEntry, configuredServers: MCPCatalogServer[])}
	{@const canConfigure = d.manifest.runtime === 'composite' || hasEditableConfiguration(d)}
	{@const requiresUpdate = configuredServers.some(requiresUserUpdate)}
	{#if canConfigure}
		<button
			class={twMerge(
				'menu-button hover:bg-surface3',
				requiresUpdate && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
			)}
			onclick={() => {
				if (configuredServers.length === 1) {
					editExistingDialog?.edit({
						server: configuredServers[0],
						entry: d
					});
				} else {
					handleShowSelectServerDialog(d, 'edit');
				}
			}}
		>
			<ServerCog class="size-4" /> Edit Configuration
		</button>
	{/if}
{/snippet}

{#snippet renameCatalogEntryAction(d: MCPCatalogEntry, configuredServers: MCPCatalogServer[])}
	<button
		class="menu-button hover:bg-surface3"
		onclick={() => {
			if (configuredServers.length === 1) {
				editExistingDialog?.rename({
					server: configuredServers[0],
					entry: d
				});
			} else {
				handleShowSelectServerDialog(d, 'rename');
			}
		}}
	>
		<PencilLine class="size-4" /> Rename
	</button>
{/snippet}

{#snippet connectToServerAction(
	d: MCPCatalogEntry | MCPCatalogServer,
	toggle: (value: boolean) => void,
	isCreateFirst?: boolean
)}
	{@const canConnect = d.canConnect !== false}
	<button
		class="menu-button disabled:cursor-not-allowed disabled:opacity-50"
		disabled={!canConnect}
		use:tooltip={{
			text: canConnect ? '' : 'See MCP Registries to grant connect access to this server',
			disablePortal: true
		}}
		onclick={async (e) => {
			e.stopPropagation();

			if ('isCatalogEntry' in d) {
				if (isCreateFirst) {
					connectToServerDialog?.open({
						entry: d
					});
				} else {
					handleShowSelectServerDialog(d);
				}
			} else {
				const entry = d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined;
				const server = 'isCatalogEntry' in d ? undefined : d;
				connectToServerDialog?.open({
					entry,
					server,
					instance: instancesMap.get(d.id)
				});
			}
			toggle(false);
		}}
	>
		<SatelliteDish class="size-4" /> Connect To Server
	</button>
{/snippet}

<McpConfirmDelete
	names={[deletingEntry?.manifest?.name ?? '']}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry) {
			return;
		}

		if (deletingEntry.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceMCPCatalogEntry(
				deletingEntry.powerUserWorkspaceID,
				deletingEntry.id
			);
		} else if (catalog) {
			await AdminService.deleteMCPCatalogEntry(catalog.id, deletingEntry.id);
		}

		await fetch();
		deletingEntry = undefined;
	}}
	oncancel={() => (deletingEntry = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={[deletingServer?.alias || deletingServer?.manifest?.name || '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) {
			return;
		}

		try {
			if (deletingServer.catalogEntryID) {
				await ChatService.deleteSingleOrRemoteMcpServer(deletingServer.id);
			} else if (deletingServer.powerUserWorkspaceID) {
				await ChatService.deleteWorkspaceMCPCatalogServer(
					deletingServer.powerUserWorkspaceID,
					deletingServer.id
				);
			} else if (catalog) {
				await AdminService.deleteMCPCatalogServer(catalog.id, deletingServer.id);
			}

			await fetch();
			deletingServer = undefined;
		} catch (error) {
			if (error instanceof MCPCompositeDeletionDependencyError) {
				deleteConflictError = error;
				return;
			}

			throw error;
		}
	}}
	oncancel={() => (deletingServer = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={Object.values(selected).map((s) => s.name)}
	show={confirmBulkDelete}
	onsuccess={async () => {
		loadingBulkDelete = true;
		try {
			for (const item of Object.values(selected)) {
				if ('isCatalogEntry' in item.data) {
					if (item.data.powerUserWorkspaceID) {
						await ChatService.deleteWorkspaceMCPCatalogEntry(
							item.data.powerUserWorkspaceID,
							item.data.id
						);
					} else if (catalog) {
						await AdminService.deleteMCPCatalogEntry(catalog.id, item.data.id);
					}
				} else if (!item.data.catalogEntryID) {
					try {
						if (item.data.powerUserWorkspaceID) {
							await ChatService.deleteWorkspaceMCPCatalogServer(
								item.data.powerUserWorkspaceID,
								item.data.id
							);
						} else if (catalog) {
							await AdminService.deleteMCPCatalogServer(catalog.id, item.data.id);
						}
					} catch (error) {
						if (error instanceof MCPCompositeDeletionDependencyError) {
							deleteConflictError = error;
							// Stop processing further deletes; user must resolve dependencies first.
							break;
						}

						throw error;
					}
				} else {
					await ChatService.deleteSingleOrRemoteMcpServer(item.data.id);
				}
			}

			await fetch();
		} finally {
			confirmBulkDelete = false;
			loadingBulkDelete = false;
		}
	}}
	oncancel={() => (confirmBulkDelete = false)}
	loading={loadingBulkDelete}
	entity="entry"
	entityPlural="entries"
/>

<McpMultiDeleteBlockedDialog
	show={!!deleteConflictError}
	error={deleteConflictError}
	onClose={() => {
		deleteConflictError = undefined;
	}}
/>

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	{onConnect}
/>

<ResponsiveDialog
	class="bg-surface1 dark:bg-background"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={async (d) => {
			selectServerDialog?.close();
			switch (selectServerMode) {
				case 'chat': {
					connectToServerDialog?.handleSetupChat(d);
					break;
				}
				case 'server-details': {
					goto(resolve(`/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`));
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
					await ChatService.deleteSingleOrRemoteMcpServer(d.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					break;
				}
				default:
					connectToServerDialog?.open({
						entry: selectedEntry,
						server: d
					});
					break;
			}
		}}
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex flex-shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{d.alias || d.manifest.name}
						{#if 'needsUpdate' in d && d.needsUpdate}
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
			<button class="icon-button hover:dark:bg-background/50">
				<StepForward class="size-4" />
			</button>
		{/snippet}
	</Table>
	{#if selectServerMode === 'connect'}
		<p class="my-4 self-center text-center text-sm font-semibold">OR</p>
		<button
			class="button-primary"
			onclick={() => {
				selectServerDialog?.close();
				connectToServerDialog?.open({
					entry: selectedEntry
				});
			}}
		>
			Connect New Server
		</button>
	{/if}
</ResponsiveDialog>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={() => {
		mcpServersAndEntries.refreshUserConfiguredServers();
	}}
/>
