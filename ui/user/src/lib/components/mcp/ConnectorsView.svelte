<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
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
		Group
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
	import {
		AlertTriangle,
		Captions,
		CircleFadingArrowUp,
		Ellipsis,
		LoaderCircle,
		MessageCircle,
		PencilLine,
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
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { twMerge } from 'tailwind-merge';
	import EditExistingDeployment from './EditExistingDeployment.svelte';

	type Item = ReturnType<typeof convertEntriesAndServersToTableData>[number];

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
		onConnect?: () => void;
		onConnectClose?: () => void;
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
		initSort,
		classes,
		onConnect,
		onConnectClose
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
			undefined,
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
				: ['name', 'connected', 'created']}
			headers={[{ title: 'Status', property: 'connected' }]}
			filterable={['name', 'type', 'registry']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				let url = '';
				let useAdminUrl =
					window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

				const id =
					'catalogEntryID' in d.data && d.data.catalogEntryID ? d.data.catalogEntryID : d.id;
				if (useAdminUrl) {
					if (d.type === 'single' || d.type === 'remote' || d.type === 'composite') {
						url = d.data.powerUserWorkspaceID
							? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${id}`
							: `/admin/mcp-servers/c/${id}`;
					} else {
						url = d.data.powerUserWorkspaceID
							? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${id}`
							: `/admin/mcp-servers/s/${id}`;
					}
				} else {
					url =
						d.type === 'single' || d.type === 'remote' || d.type === 'composite'
							? `/mcp-servers/c/${id}`
							: `/mcp-servers/s/${id}`;
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
			validateSelect={(d) => d.editable}
			disabledSelectMessage="This entry is managed by Git; changes cannot be made."
			setRowClasses={(d) => {
				const server =
					'isCatalogEntry' in d.data
						? mcpServersAndEntries.current.userConfiguredServers.find(
								(server) => server.catalogEntryID === d.data.id && !server.alias
							)
						: d.data;

				if (server) {
					return server.needsUpdate
						? 'bg-primary/10'
						: requiresUserUpdate(server)
							? 'bg-yellow-500/10'
							: '';
				}

				return '';
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{@const server =
					'isCatalogEntry' in d.data
						? mcpServersAndEntries.current.userConfiguredServers.find(
								(server) => server.catalogEntryID === d.data.id && !server.alias
							)
						: d.data}
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
							{#if server}
								{#if server.needsUpdate}
									<span
										use:tooltip={{
											classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
											text: 'An update requires your attention'
										}}
									>
										<CircleFadingArrowUp class="text-primary size-4" />
									</span>
								{:else if requiresUserUpdate(server)}
									<span
										class="text-yellow-500"
										use:tooltip={{
											text: 'Server requires an update.',
											disablePortal: true
										}}
									>
										<TriangleAlert class="size-4" />
									</span>
								{/if}
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
				{@const belongsToUser = entity === 'workspace' && id && d.data.powerUserWorkspaceID === id}
				{@const canDelete =
					d.editable && !readonly && (belongsToUser || profile.current?.hasAdminAccess?.())}
				{@const matchingServer =
					d.connected && 'isCatalogEntry' in d.data
						? mcpServersAndEntries.current.userConfiguredServers.find(
								(server) => server.catalogEntryID === d.data.id && !server.alias
							)
						: undefined}
				{@const hasConnectedOptions = 'isCatalogEntry' in d.data ? !!matchingServer : d.connected}
				<DotDotDot class="icon-button hover:dark:bg-background/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
							<button
								class="menu-button-primary"
								onclick={async (e) => {
									e.stopPropagation();

									if ('isCatalogEntry' in d.data && d.connected) {
										selectedConfiguredServers =
											mcpServersAndEntries.current.userConfiguredServers.filter(
												(server) => server.catalogEntryID === d.data.id
											);
										selectedEntry = d.data;
										selectServerDialog?.open();
									} else {
										const entry =
											'isCatalogEntry' in d.data
												? d.data
												: d.data.catalogEntryID
													? entriesMap.get(d.data.catalogEntryID)
													: undefined;
										const server = 'isCatalogEntry' in d.data ? undefined : d.data;
										connectToServerDialog?.open({
											entry,
											server
										});
									}
									toggle(false);
								}}
							>
								<SatelliteDish class="size-4" /> Connect To Server
							</button>

							{#if hasConnectedOptions}
								<button
									class="menu-button"
									onclick={async (e) => {
										e.stopPropagation();
										if ('isCatalogEntry' in d.data) {
											if (matchingServer) {
												connectToServerDialog?.handleSetupChat(matchingServer);
											}
										} else {
											connectToServerDialog?.handleSetupChat(d.data, instancesMap.get(d.id));
										}
										toggle(false);
									}}
								>
									<MessageCircle class="size-4" /> Chat
								</button>

								{@render editConfigAction('isCatalogEntry' in d.data ? matchingServer! : d.data)}
								{@render renameAction('isCatalogEntry' in d.data ? matchingServer! : d.data)}
							{/if}

							{#if auditLogUrl}
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
							{#if 'isCatalogEntry' in d.data && matchingServer}
								<button
									class="menu-button"
									onclick={async (e) => {
										e.stopPropagation();
										await ChatService.deleteSingleOrRemoteMcpServer(matchingServer.id);
										mcpServersAndEntries.current.userConfiguredServers =
											mcpServersAndEntries.current.userConfiguredServers.filter(
												(server) => server.id !== matchingServer.id
											);
										toggle(false);
									}}
								>
									<Unplug class="size-4" /> Disconnect
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
									<Trash2 class="size-4" /> Delete {'isCatalogEntry' in d.data ? 'Entry' : 'Server'}
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							confirmBulkDelete = true;
						}}
						disabled={readonly}
					>
						<Trash2 class="size-4" /> Delete
					</button>
				</div>
			{/snippet}
		</Table>
	{/if}
</div>

{#snippet editConfigAction(d: MCPCatalogServer)}
	{@const requiresUpdate = requiresUserUpdate(d)}
	{@const entry = d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined}
	{@const canConfigure =
		entry && (entry.manifest.runtime === 'composite' || hasEditableConfiguration(entry))}
	{#if canConfigure}
		<button
			class={twMerge(
				'menu-button',
				requiresUpdate && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
			)}
			onclick={() => {
				editExistingDialog?.edit({
					server: d,
					entry
				});
			}}
		>
			<ServerCog class="size-4" /> Edit Configuration
		</button>
	{/if}
{/snippet}

{#snippet renameAction(d: MCPCatalogServer)}
	<button
		class="menu-button"
		onclick={() => {
			editExistingDialog?.rename({
				server: d,
				entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
			});
		}}
	>
		<PencilLine class="size-4" /> Rename
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
	names={[deletingServer?.manifest?.name ?? '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer || !catalog) {
			return;
		}

		try {
			if (deletingServer.powerUserWorkspaceID) {
				await ChatService.deleteWorkspaceMCPCatalogServer(
					deletingServer.powerUserWorkspaceID,
					deletingServer.id
				);
			} else if (catalog && !deletingServer.catalogEntryID) {
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
	onClose={onConnectClose}
/>

<ResponsiveDialog bind:this={selectServerDialog} title="Select Your Server">
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={(d) => {
			// onConnect?.({
			// 	entry: selectedEntry,
			// 	server: d
			// });
			selectServerDialog?.close();
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
	<p class="my-4 self-center text-center text-sm font-semibold">OR</p>
	<button
		class="button-primary"
		onclick={() => {
			selectServerDialog?.close();
			// onConnect?.({
			// 	entry: selectedEntry
			// });
		}}>Connect New Server</button
	>
</ResponsiveDialog>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={() => {
		mcpServersAndEntries.refreshUserConfiguredServers();
	}}
/>
